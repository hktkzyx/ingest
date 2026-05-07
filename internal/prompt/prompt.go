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
//	Detected: SONY ZVE10M2 (zve10m2, confidence 0.80 via directory structure)
//	Accept? [Y/n/list]:
func (io IO) ConfirmDevice(name, id, reason string, confidence float64) (DeviceChoice, error) {
	fmt.Fprintf(io.Out, "Detected device: %s (%s, confidence %.2f via %s)\n",
		name, id, confidence, reason)
	fmt.Fprint(io.Out, "Accept? [Y/n/list]: ")
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
		return DeviceReject, fmt.Errorf("unrecognized answer %q (expected y/n/list)", ans)
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
		return -1, fmt.Errorf("no devices to pick from")
	}
	fmt.Fprintln(io.Out, "Devices in config:")
	for i, o := range options {
		fmt.Fprintf(io.Out, "  [%d] %-12s %s (%s)\n", i+1, o.ID, o.Name, o.Manufacturer)
	}
	fmt.Fprint(io.Out, "Pick one [1]: ")
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
		return -1, fmt.Errorf("invalid selection %q", ans)
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
//	Segment 1/3: 2026-04-27 → 2026-04-28 (123 files, 4.2 GB)
//	  Range [enter to accept | YYYY-MM-DD..YYYY-MM-DD]: <enter>
//	  Event name: 周末骑行
func (io IO) EditSegment(s SegmentEdit) (SegmentResult, error) {
	fmt.Fprintf(io.Out, "\nSegment %d/%d: %s → %s (%d files, %s)\n",
		s.Index, s.Total,
		s.Start.Format("2006-01-02"), s.End.Format("2006-01-02"),
		s.FileCount, humanBytes(s.Bytes))

	fmt.Fprint(io.Out, "  Range [enter to accept | YYYY-MM-DD..YYYY-MM-DD]: ")
	rangeLine, err := io.readLine()
	if err != nil {
		return SegmentResult{}, err
	}
	start, end := s.Start, s.End
	if t := strings.TrimSpace(rangeLine); t != "" {
		ps, pe, err := parseRange(t)
		if err != nil {
			return SegmentResult{}, fmt.Errorf("range: %w", err)
		}
		start, end = ps, pe
	}

	fmt.Fprint(io.Out, "  Event name: ")
	name, err := io.readLine()
	if err != nil {
		return SegmentResult{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return SegmentResult{}, fmt.Errorf("event name cannot be empty")
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

// humanBytes 把字节数格式成 "4.2 GB" / "123.0 MB" 这样的字符串，纯展示用。
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
