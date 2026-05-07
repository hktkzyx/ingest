package period

import (
	"io/fs"
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

// File 是 Infer 的输入：路径用来抽 EXIF/QT，Info 提供 mtime 兜底。
// 解耦 scanner 包以避免 period → scanner 的反向依赖。
type File struct {
	Path string
	Info fs.FileInfo
}

// Stats 描述时间来源分布，verbose 模式下打印用。
type Stats struct {
	Total          int
	FromExif       int // dispatch 到 EXIF 提取并成功
	FromQuickTime  int // dispatch 到 QT atom 提取并成功
	FromMtime      int // 上述失败或不支持的扩展，回退到 mtime
}

// Infer 计算文件集合的拍摄日期跨度。优先用 timestamp.Extract 读取媒体内嵌时间，
// 失败时回退到 fs.FileInfo.ModTime()。
func Infer(files []File) (Period, Stats) {
	var p Period
	var s Stats
	s.Total = len(files)
	for _, f := range files {
		t, source := resolve(f)
		switch source {
		case sourceExif:
			s.FromExif++
		case sourceQT:
			s.FromQuickTime++
		case sourceMtime:
			s.FromMtime++
		}
		if t.IsZero() {
			continue
		}
		if p.Start.IsZero() || t.Before(p.Start) {
			p.Start = t
		}
		if t.After(p.End) {
			p.End = t
		}
	}
	return p, s
}

type sourceKind int

const (
	sourceMtime sourceKind = iota
	sourceExif
	sourceQT
)

func resolve(f File) (time.Time, sourceKind) {
	if t, err := timestamp.Extract(f.Path); err == nil {
		// 区分来源仅靠扩展名足够（Extract 内部就是按扩展名分发）。
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
