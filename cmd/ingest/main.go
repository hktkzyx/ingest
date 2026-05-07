package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hktkzyx/ingest/internal/copier"
	"github.com/hktkzyx/ingest/internal/db"
	"github.com/hktkzyx/ingest/internal/device"
	"github.com/hktkzyx/ingest/internal/mount"
	"github.com/hktkzyx/ingest/internal/period"
	"github.com/hktkzyx/ingest/internal/scanner"
	"github.com/hktkzyx/ingest/internal/template"
)

var version = "0.1.0-alpha.1"

const defaultTemplate = "{date_start}[_{date_end}]-{event_name}/origin-{device_name}"

var (
	flagSource      string
	flagTarget      string
	flagName        string
	flagDevice      string
	flagFrom        string
	flagTo          string
	flagTemplate    string
	flagDBPath      string
	flagDevicesPath string
	flagDryRun      bool
	flagYes         bool
	flagVerbose     bool
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
	cmd.PersistentFlags().StringVar(&flagDevicesPath, "devices", device.DefaultConfigPath(),
		"path to devices.yaml (auto-created with built-in defaults if missing)")

	cmd.Flags().StringVarP(&flagSource, "source", "s", "", "source path (mounted volume)")
	cmd.Flags().StringVarP(&flagTarget, "target", "t", defaultTarget(), "target root directory")
	cmd.Flags().StringVarP(&flagName, "name", "n", "", "event name")
	cmd.Flags().StringVar(&flagDevice, "device", "", "force device id")
	cmd.Flags().StringVar(&flagFrom, "from", "", "period start (YYYY-MM-DD)")
	cmd.Flags().StringVar(&flagTo, "to", "", "period end (YYYY-MM-DD)")
	cmd.Flags().StringVar(&flagTemplate, "template", defaultTemplate, "path template")
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
		Short: "List configured device rules",
		RunE: func(_ *cobra.Command, _ []string) error {
			path, err := expandPath(flagDevicesPath)
			if err != nil {
				return err
			}
			rules, err := device.LoadOrInit(path)
			if err != nil {
				return err
			}
			fmt.Printf("(rules from %s)\n", path)
			for _, r := range rules {
				fmt.Printf("  %-12s %s (%s)\n", r.ID, r.Name, r.Manufacturer)
			}
			return nil
		},
	})
	return c
}

func runIngest(_ *cobra.Command, _ []string) error {
	target, err := expandPath(flagTarget)
	if err != nil {
		return fmt.Errorf("--target: %w", err)
	}
	devicesPath, err := expandPath(flagDevicesPath)
	if err != nil {
		return fmt.Errorf("--devices: %w", err)
	}
	rules, err := device.LoadOrInit(devicesPath)
	if err != nil {
		return fmt.Errorf("load devices: %w", err)
	}
	if len(rules) == 0 {
		return fmt.Errorf("no device rules in %s — add at least one entry", devicesPath)
	}

	src, err := resolveSource(rules)
	if err != nil {
		return err
	}

	files, err := scanner.Scan(src)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no media files found under %s", src)
	}
	fmt.Printf("Scanned %d files in %s\n", len(files), src)

	deviceID, deviceName, err := resolveDevice(src, rules)
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
		DeviceName: strings.ReplaceAll(deviceName, " ", "_"),
	})
	if err != nil {
		return fmt.Errorf("template: %w", err)
	}
	targetDir := filepath.Join(target, rendered)
	fmt.Printf("Target: %s\n", targetDir)

	if flagDryRun {
		for _, f := range files {
			fmt.Printf("  [dry-run] %s → %s\n",
				f.RelPath, filepath.Join(targetDir, filepath.Base(f.Path)))
		}
		return nil
	}

	dbPath, err := expandPath(flagDBPath)
	if err != nil {
		return fmt.Errorf("--db: %w", err)
	}
	store, err := db.Open(dbPath)
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

// resolveSource 决定要扫的源路径。优先级：显式 --source > 自动检测。
//
// 自动检测时枚举系统挂载点（mount.List），对每个候选跑 device.Detect 评分，
// 按置信度排序后：
//   - 0 个匹配 → 报错让用户手动指定
//   - 1 个匹配 → 直接采用
//   - N 个匹配 → 列表 + 交互式数字选择；--yes 时自动取最高分
func resolveSource(rules []device.Rule) (string, error) {
	if flagSource != "" {
		src, err := expandPath(flagSource)
		if err != nil {
			return "", fmt.Errorf("--source: %w", err)
		}
		if st, err := os.Stat(src); err != nil || !st.IsDir() {
			return "", fmt.Errorf("source not a directory: %s", src)
		}
		return src, nil
	}

	vols, err := mount.List()
	if err != nil {
		return "", fmt.Errorf("auto-detect mounts: %w (pass --source explicitly)", err)
	}
	type candidate struct {
		Volume     mount.Volume
		Match      *device.Match
		Confidence float64
	}
	var matches []candidate
	for _, v := range vols {
		m := device.Detect(rules, v.Path, v.Label)
		if m == nil {
			continue
		}
		matches = append(matches, candidate{Volume: v, Match: m, Confidence: m.Confidence})
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no removable volumes matched any device rule; pass --source explicitly")
	case 1:
		c := matches[0]
		fmt.Printf("Auto-detected source: %s (%s, confidence %.2f via %s)\n",
			c.Volume.Path, c.Match.Rule.Name, c.Confidence, c.Match.Reason)
		return c.Volume.Path, nil
	}

	// 多个候选：按置信度降序展示。
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].Confidence > matches[i].Confidence {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
	if flagYes {
		c := matches[0]
		fmt.Printf("Auto-selected (--yes): %s (%s, confidence %.2f)\n",
			c.Volume.Path, c.Match.Rule.Name, c.Confidence)
		return c.Volume.Path, nil
	}

	fmt.Println("Multiple removable volumes matched device rules:")
	for i, c := range matches {
		fmt.Printf("  [%d] %-40s %s (confidence %.2f, %s)\n",
			i+1, c.Volume.Path, c.Match.Rule.Name, c.Confidence, c.Match.Reason)
	}
	fmt.Print("Pick one [1]: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read selection: %w", err)
	}
	line = strings.TrimSpace(line)
	idx := 1
	if line != "" {
		n, err := strconv.Atoi(line)
		if err != nil || n < 1 || n > len(matches) {
			return "", fmt.Errorf("invalid selection %q", line)
		}
		idx = n
	}
	return matches[idx-1].Volume.Path, nil
}

func resolveDevice(src string, rules []device.Rule) (id, name string, err error) {
	if flagDevice != "" {
		for _, r := range rules {
			if r.ID == flagDevice {
				return r.ID, r.Name, nil
			}
		}
		return flagDevice, flagDevice, nil
	}
	label := volumeLabelOf(src)
	m := device.Detect(rules, src, label)
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
	pfiles := make([]period.File, 0, len(files))
	for _, f := range files {
		pfiles = append(pfiles, period.File{Path: f.Path, Info: f.Info})
	}
	p, stats := period.Infer(pfiles)
	if flagVerbose {
		fmt.Printf("(time source: exif=%d quicktime=%d mtime-fallback=%d)\n",
			stats.FromExif, stats.FromQuickTime, stats.FromMtime)
	}
	return p, nil
}

func optionalEnd(p period.Period) string {
	if p.IsSingleDay() {
		return ""
	}
	return p.End.Format("20060102")
}

// expandPath 把用户在 CLI 上输入的任意风格路径标准化：
//   - 展开开头的 ~（当前用户家目录）
//   - 展开 $VAR 形式的环境变量
//   - 解析为绝对路径并 Clean
//
// Windows 上 filepath 包原生处理 `\`、盘符、UNC 路径，无需额外分支。
// `%VAR%` 风格的 Windows 环境变量不展开（与 Go 标准做法一致；
// 若需要，文档建议改写成 `$VAR`）。
func expandPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand ~: %w", err)
		}
		p = filepath.Join(home, strings.TrimLeft(p[1:], `/\`))
	}
	p = os.ExpandEnv(p)
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func defaultTarget() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Backups")
}

func defaultDB() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ingest", "ingest.db")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "ingest", "ingest.db")
}
