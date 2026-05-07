package period

import (
	"io/fs"
	"path/filepath"
	"testing"
	"time"
)

type fakeInfo struct {
	mtime time.Time
}

func (f fakeInfo) Name() string       { return "fake" }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return 0 }
func (f fakeInfo) ModTime() time.Time { return f.mtime }
func (f fakeInfo) IsDir() bool        { return false }
func (f fakeInfo) Sys() any           { return nil }

func TestInfer_FallsBackToMtimeForUnsupportedExt(t *testing.T) {
	dir := t.TempDir()
	t1 := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 28, 11, 0, 0, 0, time.UTC)

	// 用 .txt 扩展名保证 timestamp.Extract 必然返回 ErrUnsupported，从而走 mtime。
	files := []File{
		{Path: filepath.Join(dir, "a.txt"), Info: fakeInfo{mtime: t1}},
		{Path: filepath.Join(dir, "b.txt"), Info: fakeInfo{mtime: t2}},
	}
	p, s := Infer(files)
	if !p.Start.Equal(t1) || !p.End.Equal(t2) {
		t.Fatalf("period mismatch: got [%s, %s]", p.Start, p.End)
	}
	if s.FromMtime != 2 || s.FromExif != 0 || s.FromQuickTime != 0 {
		t.Fatalf("stats mismatch: %+v", s)
	}
}

func TestInfer_EmptyInputsReturnsZeroPeriod(t *testing.T) {
	p, s := Infer(nil)
	if !p.Start.IsZero() || !p.End.IsZero() {
		t.Fatalf("expected zero period, got [%s, %s]", p.Start, p.End)
	}
	if s.Total != 0 {
		t.Fatalf("expected zero stats, got %+v", s)
	}
}

func TestIsSingleDay(t *testing.T) {
	day := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		p    Period
		want bool
	}{
		{"both zero", Period{}, true},
		{"same day diff time", Period{Start: day.Add(time.Hour), End: day.Add(5 * time.Hour)}, true},
		{"different days", Period{Start: day, End: day.Add(48 * time.Hour)}, false},
	}
	for _, c := range cases {
		if got := c.p.IsSingleDay(); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

