package period

import (
	"io/fs"
	"time"
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

// Infer derives the date range from file mtimes.
// EXIF/QuickTime extraction is a future enhancement; mtime is the safe fallback.
func Infer(infos []fs.FileInfo) Period {
	var p Period
	for _, info := range infos {
		t := info.ModTime()
		if p.Start.IsZero() || t.Before(p.Start) {
			p.Start = t
		}
		if t.After(p.End) {
			p.End = t
		}
	}
	return p
}
