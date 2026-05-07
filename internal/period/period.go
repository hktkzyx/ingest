package period

import (
	"io/fs"
	"sort"
	"time"

	"github.com/hktkzyx/ingest/internal/timestamp"
)

type Period struct {
	Start time.Time
	End   time.Time
}

func (p Period) IsSingleDay() bool {
	if p.Start.IsZero() || p.End.IsZero() {
		return true
	}
	sy, sm, sd := p.Start.Date()
	ey, em, ed := p.End.Date()
	return sy == ey && sm == em && sd == ed
}

// File 是 Infer/Segments 的输入：路径用来抽 EXIF/QT，Info 提供 mtime 兜底。
// 解耦 scanner 包以避免 period → scanner 的反向依赖。
type File struct {
	Path string
	Info fs.FileInfo
}

// Segment 是一组按时间相邻的文件，对应一次"事件"（一次拍摄活动）。
// Files 已按时间升序排序；Start / End 是 Files 中最早 / 最晚的拍摄日期。
type Segment struct {
	Start time.Time
	End   time.Time
	Files []File
	Bytes int64 // 该段所有文件字节数之和，便于 prompt 展示
}

// Period 把 Segment 转成原有的 Period 结构体（方便复用 IsSingleDay 等逻辑）。
func (s Segment) Period() Period {
	return Period{Start: s.Start, End: s.End}
}

// Stats 描述时间来源分布，verbose 模式下打印用。
type Stats struct {
	Total         int
	FromExif      int // dispatch 到 EXIF 提取并成功
	FromQuickTime int // dispatch 到 QT atom 提取并成功
	FromMtime     int // 上述失败或不支持的扩展，回退到 mtime
}

// Segments 把文件按拍摄时间分组成"事件段"。
//
// 算法：
//  1. 解析每个文件的时间（EXIF/QT 优先，mtime 兜底）
//  2. 按时间升序排列
//  3. 相邻两文件的拍摄日期间隔（按"天"算，忽略小时/分钟）≤ gapDays 视为
//     同段；否则起新段。gapDays = 0 表示必须同一天才算同段。
//
// 注意：日期间隔按"日历天"计算而非"24 小时"——21:00 拍的与第二天 06:00
// 拍的间隔是 1 天而非 9 小时；gapDays = 1 时它们同段。
func Segments(files []File, gapDays int) ([]Segment, Stats) {
	if len(files) == 0 {
		return nil, Stats{}
	}
	if gapDays < 0 {
		gapDays = 0
	}

	type entry struct {
		file File
		t    time.Time
	}
	resolved := make([]entry, 0, len(files))
	var stats Stats
	stats.Total = len(files)
	for _, f := range files {
		t, source := resolve(f)
		switch source {
		case sourceExif:
			stats.FromExif++
		case sourceQT:
			stats.FromQuickTime++
		case sourceMtime:
			stats.FromMtime++
		}
		if t.IsZero() {
			continue
		}
		resolved = append(resolved, entry{file: f, t: t})
	}
	if len(resolved) == 0 {
		return nil, stats
	}
	sort.Slice(resolved, func(i, j int) bool { return resolved[i].t.Before(resolved[j].t) })

	var out []Segment
	current := Segment{}
	startNew := func(e entry) {
		current = Segment{
			Start: dayOf(e.t),
			End:   dayOf(e.t),
			Files: []File{e.file},
			Bytes: sizeOf(e.file),
		}
	}
	startNew(resolved[0])
	for i := 1; i < len(resolved); i++ {
		gap := daysBetween(resolved[i-1].t, resolved[i].t)
		if gap <= gapDays {
			current.Files = append(current.Files, resolved[i].file)
			current.End = dayOf(resolved[i].t)
			current.Bytes += sizeOf(resolved[i].file)
			continue
		}
		out = append(out, current)
		startNew(resolved[i])
	}
	out = append(out, current)
	return out, stats
}

// Infer 是 Segments 的退化版：把所有文件视为单段，返回该段的 [Start, End]。
// 保留给 --from / --to 模式或不需要分段的调用方使用。
func Infer(files []File) (Period, Stats) {
	segs, stats := Segments(files, 365*100) // 给个超大 gap，强制单段
	if len(segs) == 0 {
		return Period{}, stats
	}
	return segs[0].Period(), stats
}

type sourceKind int

const (
	sourceMtime sourceKind = iota
	sourceExif
	sourceQT
)

func resolve(f File) (time.Time, sourceKind) {
	if t, err := timestamp.Extract(f.Path); err == nil {
		if isVideoExt(f.Path) {
			return t, sourceQT
		}
		return t, sourceExif
	}
	if f.Info != nil {
		return f.Info.ModTime(), sourceMtime
	}
	return time.Time{}, sourceMtime
}

func sizeOf(f File) int64 {
	if f.Info == nil {
		return 0
	}
	return f.Info.Size()
}

// dayOf 把时间归一化到当天 00:00:00（本地时区）。
// 用本地时区是因为 EXIF DateTimeOriginal 本来就是本地拍摄时间，
// 用户的目录命名习惯也按本地日历。
func dayOf(t time.Time) time.Time {
	y, m, d := t.In(time.Local).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

func daysBetween(a, b time.Time) int {
	delta := dayOf(b).Sub(dayOf(a))
	if delta < 0 {
		delta = -delta
	}
	return int(delta / (24 * time.Hour))
}

func isVideoExt(path string) bool {
	for i := len(path) - 1; i >= 0 && path[i] != '/' && path[i] != '\\'; i-- {
		if path[i] == '.' {
			ext := lower(path[i:])
			switch ext {
			case ".mp4", ".mov", ".m4v", ".mts", ".m2ts":
				return true
			}
			return false
		}
	}
	return false
}

func lower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
