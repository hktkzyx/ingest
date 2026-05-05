package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hktkzyx/ingest/internal/copier"
	"github.com/hktkzyx/ingest/internal/db"
	"github.com/hktkzyx/ingest/internal/device"
	"github.com/hktkzyx/ingest/internal/period"
	"github.com/hktkzyx/ingest/internal/scanner"
	"github.com/hktkzyx/ingest/internal/template"
)

var version = "0.0.1-dev"

var (
	flagSource   string
	flagTarget   string
	flagName     string
	flagDevice   string
	flagFrom     string
	flagTo       string
	flagTemplate string
	flagDBPath   string
	flagDryRun   bool
	flagYes      bool
	flagVerbose  bool
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Smart media ingestion tool",
		RunE:  runIngest,
	}
	cmd.Flags().StringVarP(&flagSource, "source", "s", "", "source path (mounted volume)")
	cmd.Flags().StringVarP(&flagTarget, "target", "t", defaultTarget(), "target root directory")
	cmd.Flags().StringVarP(&flagName, "name", "n", "", "event name")
	cmd.Flags().StringVar(&flagDevice, "device", "", "force device id")
	cmd.Flags().StringVar(&flagFrom, "from", "", "period start (YYYY-MM-DD)")
	cmd.Flags().StringVar(&flagTo, "to", "", "period end (YYYY-MM-DD)")
	cmd.Flags().StringVar(&flagTemplate, "template",
		"{date_start}[_{date_end}]-{event_name}/origin-{device_id}", "path template")
	cmd.Flags().StringVar(&flagDBPath, "db", defaultDB(), "history database path")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "preview only, do not copy")
	cmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "skip confirmation prompts")
	cmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")

	cmd.AddCommand(versionCmd(), devicesCmd())
	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println("ingest", version)
		},
	}
}

func devicesCmd() *cobra.Command {
	c := &cobra.Command{Use: "devices", Short: "Manage device rules"}
	c.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List built-in device rules",
		Run: func(_ *cobra.Command, _ []string) {
			for _, r := range device.BuiltinRules {
				fmt.Printf("%-12s %s (%s)\n", r.ID, r.Name, r.Manufacturer)
			}
		},
	})
	return c
}

func runIngest(_ *cobra.Command, _ []string) error {
	if flagSource == "" {
		return fmt.Errorf("--source is required (auto-detection of mounted volumes is not implemented yet)")
	}
	src, err := filepath.Abs(flagSource)
	if err != nil {
		return err
	}
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return fmt.Errorf("source not a directory: %s", src)
	}

	files, err := scanner.Scan(src)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no media files found under %s", src)
	}
	fmt.Printf("Scanned %d files in %s\n", len(files), src)

	deviceID, deviceName, err := resolveDevice(src)
	if err != nil {
		return err
	}
	fmt.Printf("Device: %s (%s)\n", deviceID, deviceName)

	p, err := resolvePeriod(files)
	if err != nil {
		return err
	}
	fmt.Printf("Period: %s → %s\n",
		p.Start.Format("2006-01-02"), p.End.Format("2006-01-02"))

	if flagName == "" {
		return fmt.Errorf("--name is required (interactive prompt not implemented yet)")
	}

	rendered, err := template.Render(flagTemplate, template.Context{
		DateStart:  p.Start.Format("20060102"),
		DateEnd:    optionalEnd(p),
		EventName:  flagName,
		DeviceID:   deviceID,
		DeviceName: deviceName,
	})
	if err != nil {
		return fmt.Errorf("template: %w", err)
	}
	targetDir := filepath.Join(flagTarget, rendered)
	fmt.Printf("Target: %s\n", targetDir)

	if flagDryRun {
		for _, f := range files {
			fmt.Printf("  [dry-run] %s → %s\n",
				f.RelPath, filepath.Join(targetDir, filepath.Base(f.Path)))
		}
		return nil
	}

	store, err := db.Open(flagDBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer store.Close()

	var copied, skipped, failed int
	var totalBytes int64
	start := time.Now()
	for _, f := range files {
		dst := filepath.Join(targetDir, filepath.Base(f.Path))
		out := copier.SafeCopy(f.Path, dst, deviceID, "", store)
		switch out.Result {
		case copier.ResultCopied:
			copied++
			totalBytes += out.Bytes
			if flagVerbose {
				fmt.Printf("  copied  %s (%s)\n", f.RelPath, out.Hash)
			}
		case copier.ResultSkipped:
			skipped++
			if flagVerbose {
				fmt.Printf("  skip    %s\n", f.RelPath)
			}
		case copier.ResultFailed:
			failed++
			fmt.Fprintf(os.Stderr, "  FAILED  %s: %v\n", f.RelPath, out.Err)
		}
	}
	elapsed := time.Since(start).Round(time.Millisecond)
	mbps := 0.0
	if elapsed > 0 {
		mbps = float64(totalBytes) / elapsed.Seconds() / (1024 * 1024)
	}
	fmt.Printf("\nDone: %d copied, %d skipped, %d failed in %s (%.1f MB/s)\n",
		copied, skipped, failed, elapsed, mbps)
	if failed > 0 {
		return fmt.Errorf("%d files failed", failed)
	}
	return nil
}

func resolveDevice(src string) (id, name string, err error) {
	if flagDevice != "" {
		for _, r := range device.BuiltinRules {
			if r.ID == flagDevice {
				return r.ID, r.Name, nil
			}
		}
		return flagDevice, flagDevice, nil
	}
	label := volumeLabelOf(src)
	m := device.Detect(src, label)
	if m == nil {
		return "", "", fmt.Errorf("could not auto-detect device; pass --device explicitly")
	}
	if flagVerbose {
		fmt.Printf("(matched by %s, confidence %.2f)\n", m.Reason, m.Confidence)
	}
	return m.Rule.ID, m.Rule.Name, nil
}

func volumeLabelOf(p string) string {
	return filepath.Base(strings.TrimRight(p, string(os.PathSeparator)))
}

func resolvePeriod(files []scanner.File) (period.Period, error) {
	if flagFrom != "" || flagTo != "" {
		var p period.Period
		var err error
		if flagFrom != "" {
			if p.Start, err = time.Parse("2006-01-02", flagFrom); err != nil {
				return p, fmt.Errorf("--from: %w", err)
			}
		}
		if flagTo != "" {
			if p.End, err = time.Parse("2006-01-02", flagTo); err != nil {
				return p, fmt.Errorf("--to: %w", err)
			}
		} else {
			p.End = p.Start
		}
		if p.Start.IsZero() {
			p.Start = p.End
		}
		return p, nil
	}
	infos := make([]fs.FileInfo, 0, len(files))
	for _, f := range files {
		infos = append(infos, f.Info)
	}
	return period.Infer(infos), nil
}

func optionalEnd(p period.Period) string {
	if p.IsSingleDay() {
		return ""
	}
	return p.End.Format("20060102")
}

func defaultTarget() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Backups")
}

func defaultDB() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "ingest", "ingest.db")
}
