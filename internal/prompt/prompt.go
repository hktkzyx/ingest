// Package prompt 集中放 ingest 用到的所有交互式 stdin 提示。
//
// 设计原则：
//   - 所有函数接收 io.Reader（输入）和 io.Writer（输出），不依赖 os.Stdin/Stdout，
//     便于测试和未来切到 TUI；调用方在 main.go 用 NewStdio() 一次性构造默认实例
//   - 单一返回错误：解析失败 / 用户主动取消（ctrl+D 等）
//   - 跨段循环、设备复用等流程编排逻辑放在 cmd/ingest，本包只管"问一个问题"
package prompt

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// IO 把读写两端打包到一起，方便传参和 mock。读取统一走 br（同一个
// bufio.Reader），避免多次构造时 buffer 重置吞掉用户后续输入。
type IO struct {
	br  *bufio.Reader
	Out io.Writer
}

// NewStdio 返回从 in 读、向 out 写的实例。
func NewStdio(in io.Reader, out io.Writer) IO {
	return IO{br: bufio.NewReader(in), Out: out}
}

// readLine 读取一行（不含换行符）。EOF 时返回空字符串和 io.EOF；
// 其它错误原样返回。
func (io IO) readLine() (string, error) {
	line, err := io.br.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	if err != nil && line == "" {
		return "", err
	}
	return line, nil
}

// DeviceChoice 描述设备 prompt 的可能结果。
type DeviceChoice int

const (
	// DeviceAccept 表示用户接受了自动检测的设备。
	DeviceAccept DeviceChoice = iota
	// DeviceList 表示用户想浏览设备列表并改选。
	DeviceList
	// DeviceReject 表示用户拒绝当前检测，但也没选别的——上层应当报错让用户加 --device。
	DeviceReject
)

// ConfirmDevice 显示自动检测的设备名并询问是否接受。返回 DeviceChoice。
//
//	检测到设备: SONY ZVE10M2 (zve10m2, 置信度 0.80, 来自 directory structure)
//	接受? [Y=接受 / n=拒绝 / list=查看列表]:
func (io IO) ConfirmDevice(name, id, reason string, confidence float64) (DeviceChoice, error) {
	fmt.Fprintf(io.Out, "检测到设备: %s (%s, 置信度 %.2f, 来自 %s)\n",
		name, id, confidence, reason)
	fmt.Fprint(io.Out, "接受? [Y=接受 / n=拒绝 / list=查看列表]: ")
	ans, err := io.readLine()
	if err != nil {
		return DeviceReject, err
	}
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "", "y", "yes":
		return DeviceAccept, nil
	case "n", "no":
		return DeviceReject, nil
	case "l", "list":
		return DeviceList, nil
	default:
		return DeviceReject, fmt.Errorf("无法识别的回答 %q (期望 y/n/list)", ans)
	}
}

// DeviceOption 是 PickDevice 显示给用户的选项。
type DeviceOption struct {
	ID           string
	Name         string
	Manufacturer string
}

// PickDevice 列出全部可选设备并按数字让用户选。返回选中的索引。
func (io IO) PickDevice(options []DeviceOption) (int, error) {
	if len(options) == 0 {
		return -1, fmt.Errorf("没有可选设备")
	}
	fmt.Fprintln(io.Out, "配置中的设备:")
	for i, o := range options {
		fmt.Fprintf(io.Out, "  [%d] %-12s %s (%s)\n", i+1, o.ID, o.Name, o.Manufacturer)
	}
	fmt.Fprint(io.Out, "选一个 [默认 1]: ")
	ans, err := io.readLine()
	if err != nil {
		return -1, err
	}
	ans = strings.TrimSpace(ans)
	if ans == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(ans)
	if err != nil || n < 1 || n > len(options) {
		return -1, fmt.Errorf("无效选择 %q", ans)
	}
	return n - 1, nil
}

// SegmentEdit 是单段交互编辑的输入/输出。Start/End 为默认值（来自分段算法），
// 用户可以回车接受或输入 YYYY-MM-DD..YYYY-MM-DD 改写。
type SegmentEdit struct {
	Index, Total int
	Start, End   time.Time
	FileCount    int
	Bytes        int64
}

// SegmentResult 是用户编辑后的结果。
type SegmentResult struct {
	Start, End time.Time
	Name       string
}

// EditSegment 展示一个段的默认范围并询问范围调整 + 事件名。
//
//	事件 1/3: 2026-04-27 → 2026-04-28 (123 个文件, 4.2 GB)
//	  日期范围 [回车接受默认 | YYYY-MM-DD..YYYY-MM-DD]: <enter>
//	  事件名称: 周末骑行
func (io IO) EditSegment(s SegmentEdit) (SegmentResult, error) {
	fmt.Fprintf(io.Out, "\n事件 %d/%d: %s → %s (%d 个文件, %s)\n",
		s.Index, s.Total,
		s.Start.Format("2006-01-02"), s.End.Format("2006-01-02"),
		s.FileCount, humanBytes(s.Bytes))

	fmt.Fprint(io.Out, "  日期范围 [回车接受默认 | YYYY-MM-DD..YYYY-MM-DD]: ")
	rangeLine, err := io.readLine()
	if err != nil {
		return SegmentResult{}, err
	}
	start, end := s.Start, s.End
	if t := strings.TrimSpace(rangeLine); t != "" {
		ps, pe, err := parseRange(t)
		if err != nil {
			return SegmentResult{}, fmt.Errorf("日期范围: %w", err)
		}
		start, end = ps, pe
	}

	fmt.Fprint(io.Out, "  事件名称: ")
	name, err := io.readLine()
	if err != nil {
		return SegmentResult{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return SegmentResult{}, fmt.Errorf("事件名称不能为空")
	}
	return SegmentResult{Start: start, End: end, Name: name}, nil
}

// parseRange 解析 "YYYY-MM-DD..YYYY-MM-DD" 或单个 "YYYY-MM-DD"。
func parseRange(s string) (start, end time.Time, err error) {
	parts := strings.SplitN(s, "..", 2)
	start, err = time.ParseInLocation("2006-01-02", strings.TrimSpace(parts[0]), time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if len(parts) == 1 {
		return start, start, nil
	}
	end, err = time.ParseInLocation("2006-01-02", strings.TrimSpace(parts[1]), time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("end %s is before start %s", end.Format("2006-01-02"), start.Format("2006-01-02"))
	}
	return start, end, nil
}

// AskTarget 询问目标根目录，回车接受 defaultDir。
//
//	保存到哪里? [默认: ~/Backups]:
func (io IO) AskTarget(defaultDir string) (string, error) {
	fmt.Fprintf(io.Out, "保存到哪里? [默认: %s]: ", defaultDir)
	line, err := io.readLine()
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultDir, nil
	}
	return line, nil
}

// ProceedSummary 拷贝前的总览数据。Segments 是每段简述（"2026-04-27→04-28 周末骑行 (123 files, 4.2 GB)"）。
type ProceedSummary struct {
	Target     string
	Device     string
	TotalFiles int
	TotalBytes int64
	Segments   []string
	DryRun     bool
}

// ConfirmProceed 在拷贝前展示总览并询问是否继续。
//
//	准备拷贝 4 个文件 (1.2 GB) → /tmp/out
//	设备: SONY ZVE10M2
//	  • 2026-04-27→04-28  五一假期前  (2 files, 580 MB)
//	  • 2026-04-30→05-01  五一假期    (2 files, 640 MB)
//	继续? [Y/n]:
func (io IO) ConfirmProceed(s ProceedSummary) (bool, error) {
	verb := "拷贝"
	if s.DryRun {
		verb = "预览（dry-run）"
	}
	fmt.Fprintf(io.Out, "\n准备%s %d 个文件 (%s) → %s\n",
		verb, s.TotalFiles, humanBytes(s.TotalBytes), s.Target)
	if s.Device != "" {
		fmt.Fprintf(io.Out, "设备: %s\n", s.Device)
	}
	for _, seg := range s.Segments {
		fmt.Fprintf(io.Out, "  • %s\n", seg)
	}
	fmt.Fprint(io.Out, "继续? [Y/n]: ")
	ans, err := io.readLine()
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "", "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("无法识别的回答 %q (期望 y/n)", ans)
	}
}

// ConflictInfo 描述一次"目标已存在但内容实质不同"的冲突现场。
// SrcHash/DstHash 在同 size 不同 hash 的情形下非空；size 不同时为空字符串
// （上层只展示 size 差就够了，不必为决策再算一次 hash）。
type ConflictInfo struct {
	RelPath          string // 相对源根的路径，给用户看
	TrashPath        string // 旧文件即将被搬到的位置（绝对或相对路径都行，纯展示）
	SrcSize, DstSize int64
	SrcHash, DstHash string
}

// ConfirmOverwrite 询问用户是否覆盖已存在但内容不同的目标文件。
// 返回 true 表示同意覆盖；false 表示保留旧目标、跳过这一份 src。
//
//	冲突: PRIVATE/M4ROOT/CLIP/C0001.MP4
//	  源:    abc123de... (12.3 MB)
//	  目标:  98ef76cd... (12.3 MB)
//	  旧文件将移到: /backups/.ingest-trash/2026-05-10T12-34-56/C0001.MP4
//	覆盖? [y/N]:
func (io IO) ConfirmOverwrite(c ConflictInfo) (bool, error) {
	fmt.Fprintf(io.Out, "\n冲突: %s\n", c.RelPath)
	if c.SrcHash != "" && c.DstHash != "" {
		fmt.Fprintf(io.Out, "  源:    %s (%s)\n", shortHash(c.SrcHash), humanBytes(c.SrcSize))
		fmt.Fprintf(io.Out, "  目标:  %s (%s)\n", shortHash(c.DstHash), humanBytes(c.DstSize))
	} else {
		fmt.Fprintf(io.Out, "  源:    %s\n", humanBytes(c.SrcSize))
		fmt.Fprintf(io.Out, "  目标:  %s (大小不同)\n", humanBytes(c.DstSize))
	}
	if c.TrashPath != "" {
		fmt.Fprintf(io.Out, "  旧文件将移到: %s\n", c.TrashPath)
	}
	fmt.Fprint(io.Out, "覆盖? [y/N]: ")
	ans, err := io.readLine()
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "y", "yes":
		return true, nil
	case "", "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("无法识别的回答 %q (期望 y/n)", ans)
	}
}

func shortHash(h string) string {
	if len(h) <= 8 {
		return h
	}
	return h[:8] + "..."
}

// HumanBytes 把字节数格式成 "4.2 GB" / "123.0 MB" 这样的字符串，纯展示用。
// 导出给 cmd 层拼 ProceedSummary.Segments 字符串用。
func HumanBytes(n int64) string { return humanBytes(n) }

func humanBytes(n int64) string {
	const unit = 1024.0
	if n < int64(unit) {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		f /= unit
		if f < unit {
			return fmt.Sprintf("%.1f %s", f, suffix)
		}
	}
	return fmt.Sprintf("%.1f PB", f/unit)
}
