package period

import (
	"io/fs"
	"path/filepath"
	"testing"
	"time"
)

type fakeInfo struct {
	size  int64
	mtime time.Time
}

func (f fakeInfo) Name() string       { return "fake" }
func (f fakeInfo) Size() int64        { return f.size }
func (f fakeInfo) Mode() fs.FileMode  { return 0 }
func (f fakeInfo) ModTime() time.Time { return f.mtime }
func (f fakeInfo) IsDir() bool        { return false }
func (f fakeInfo) Sys() any           { return nil }

// sameDay 检查两个时间在本地时区是否同一日历天。
// Segments/Infer 把 Start/End 归一化到当天 00:00:00 本地时区，所以测试
// 也用日历天比较而非精确时间。
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.In(time.Local).Date()
	by, bm, bd := b.In(time.Local).Date()
	return ay == by && am == bm && ad == bd
}

func TestInfer_FallsBackToMtimeForUnsupportedExt(t *testing.T) {
	dir := t.TempDir()
	t1 := time.Date(2026, 4, 27, 10, 0, 0, 0, time.Local)
	t2 := time.Date(2026, 4, 28, 11, 0, 0, 0, time.Local)

	// 用 .txt 扩展名保证 timestamp.Extract 必然返回 ErrUnsupported，从而走 mtime。
	files := []File{
		{Path: filepath.Join(dir, "a.txt"), Info: fakeInfo{mtime: t1}},
		{Path: filepath.Join(dir, "b.txt"), Info: fakeInfo{mtime: t2}},
	}
	p, s := Infer(files)
	if !sameDay(p.Start, t1) || !sameDay(p.End, t2) {
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

func makeFile(dir, name string, t time.Time, size int64) File {
	return File{
		Path: filepath.Join(dir, name),
		Info: fakeInfo{size: size, mtime: t},
	}
}

func TestSegments_GroupsConsecutiveDaysWithinGap(t *testing.T) {
	dir := t.TempDir()
	day := func(d int) time.Time { return time.Date(2026, 4, d, 12, 0, 0, 0, time.Local) }

	files := []File{
		makeFile(dir, "a.txt", day(27), 100), // 段 1
		makeFile(dir, "b.txt", day(28), 200), // 段 1（gap=1，合并）
		makeFile(dir, "c.txt", day(30), 300), // 段 2（gap=2 > 1，新段）
		makeFile(dir, "d.txt", day(30), 400), // 段 2
	}

	segs, _ := Segments(files, 1)
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments with gapDays=1, got %d", len(segs))
	}
	if !sameDay(segs[0].Start, day(27)) || !sameDay(segs[0].End, day(28)) {
		t.Errorf("segment 0 dates mismatch: %s..%s", segs[0].Start, segs[0].End)
	}
	if segs[0].Bytes != 300 || len(segs[0].Files) != 2 {
		t.Errorf("segment 0 byte/file count mismatch: bytes=%d files=%d", segs[0].Bytes, len(segs[0].Files))
	}
	if !sameDay(segs[1].Start, day(30)) || !sameDay(segs[1].End, day(30)) {
		t.Errorf("segment 1 dates mismatch: %s..%s", segs[1].Start, segs[1].End)
	}
	if segs[1].Bytes != 700 || len(segs[1].Files) != 2 {
		t.Errorf("segment 1 byte/file count mismatch: bytes=%d files=%d", segs[1].Bytes, len(segs[1].Files))
	}
}

func TestSegments_LargerGapMergesAcrossDates(t *testing.T) {
	dir := t.TempDir()
	day := func(d int) time.Time { return time.Date(2026, 4, d, 12, 0, 0, 0, time.Local) }

	files := []File{
		makeFile(dir, "a.txt", day(27), 1),
		makeFile(dir, "b.txt", day(30), 1), // gap=3
	}

	if segs, _ := Segments(files, 3); len(segs) != 1 {
		t.Errorf("gapDays=3 should merge, got %d segments", len(segs))
	}
	if segs, _ := Segments(files, 2); len(segs) != 2 {
		t.Errorf("gapDays=2 should split, got %d segments", len(segs))
	}
}

func TestSegments_GapZeroRequiresSameDay(t *testing.T) {
	dir := t.TempDir()
	files := []File{
		makeFile(dir, "a.txt", time.Date(2026, 4, 27, 23, 59, 0, 0, time.Local), 1),
		makeFile(dir, "b.txt", time.Date(2026, 4, 28, 0, 1, 0, 0, time.Local), 1),
	}
	segs, _ := Segments(files, 0)
	if len(segs) != 2 {
		t.Errorf("gapDays=0 should split across midnight, got %d segments", len(segs))
	}
}

func TestSegments_UnsortedInputIsSorted(t *testing.T) {
	dir := t.TempDir()
	day := func(d int) time.Time { return time.Date(2026, 4, d, 12, 0, 0, 0, time.Local) }
	files := []File{
		makeFile(dir, "later.txt", day(28), 1),
		makeFile(dir, "earlier.txt", day(27), 1),
	}
	segs, _ := Segments(files, 1)
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}
	if !sameDay(segs[0].Start, day(27)) || !sameDay(segs[0].End, day(28)) {
		t.Errorf("expected start=27 end=28, got %s..%s", segs[0].Start, segs[0].End)
	}
}
