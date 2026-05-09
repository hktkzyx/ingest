package prompt

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func newIO(input string) (IO, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return NewStdio(strings.NewReader(input), out), out
}

func TestConfirmDevice_AcceptsEmptyAndYAndYes(t *testing.T) {
	for _, in := range []string{"\n", "y\n", "Yes\n"} {
		io, _ := newIO(in)
		got, err := io.ConfirmDevice("X", "x", "directory", 0.8)
		if err != nil {
			t.Fatalf("input %q: %v", in, err)
		}
		if got != DeviceAccept {
			t.Errorf("input %q: want Accept, got %v", in, got)
		}
	}
}

func TestConfirmDevice_RejectAndList(t *testing.T) {
	io, _ := newIO("n\n")
	if got, err := io.ConfirmDevice("X", "x", "r", 0.8); err != nil || got != DeviceReject {
		t.Errorf("n: got %v err %v", got, err)
	}
	io, _ = newIO("list\n")
	if got, err := io.ConfirmDevice("X", "x", "r", 0.8); err != nil || got != DeviceList {
		t.Errorf("list: got %v err %v", got, err)
	}
}

func TestPickDevice_DefaultIsFirst(t *testing.T) {
	io, _ := newIO("\n")
	idx, err := io.PickDevice([]DeviceOption{
		{ID: "a", Name: "A"}, {ID: "b", Name: "B"},
	})
	if err != nil || idx != 0 {
		t.Errorf("default pick: got %d err %v", idx, err)
	}
}

func TestPickDevice_PicksByNumber(t *testing.T) {
	io, _ := newIO("2\n")
	idx, err := io.PickDevice([]DeviceOption{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	})
	if err != nil || idx != 1 {
		t.Errorf("pick 2: got %d err %v", idx, err)
	}
}

func TestPickDevice_RejectsOutOfRange(t *testing.T) {
	io, _ := newIO("99\n")
	if _, err := io.PickDevice([]DeviceOption{{ID: "a"}}); err == nil {
		t.Errorf("expected error for out-of-range pick")
	}
}

func TestEditSegment_AcceptDefaultsAndUseProvidedName(t *testing.T) {
	io, _ := newIO("\n周末骑行\n")
	s := SegmentEdit{
		Index: 1, Total: 1,
		Start:     time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local),
		End:       time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local),
		FileCount: 3, Bytes: 4_200_000_000,
	}
	r, err := io.EditSegment(s)
	if err != nil {
		t.Fatalf("EditSegment: %v", err)
	}
	if !r.Start.Equal(s.Start) || !r.End.Equal(s.End) {
		t.Errorf("range mismatch: got [%s,%s]", r.Start, r.End)
	}
	if r.Name != "周末骑行" {
		t.Errorf("name mismatch: %q", r.Name)
	}
}

func TestEditSegment_OverridesRange(t *testing.T) {
	io, _ := newIO("2026-05-01..2026-05-03\n五一假期\n")
	s := SegmentEdit{
		Index: 1, Total: 1,
		Start: time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local),
		End:   time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local),
	}
	r, err := io.EditSegment(s)
	if err != nil {
		t.Fatalf("EditSegment: %v", err)
	}
	wantStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	wantEnd := time.Date(2026, 5, 3, 0, 0, 0, 0, time.Local)
	if !r.Start.Equal(wantStart) || !r.End.Equal(wantEnd) {
		t.Errorf("range override failed: got [%s,%s]", r.Start, r.End)
	}
	if r.Name != "五一假期" {
		t.Errorf("name mismatch: %q", r.Name)
	}
}

func TestEditSegment_RejectsEmptyName(t *testing.T) {
	io, _ := newIO("\n\n")
	s := SegmentEdit{Index: 1, Total: 1, Start: time.Now(), End: time.Now()}
	_, err := io.EditSegment(s)
	if err == nil {
		t.Fatalf("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "事件名称") {
		t.Errorf("expected Chinese error about empty name, got %q", err.Error())
	}
}

func TestEditSegment_SingleDateAsRange(t *testing.T) {
	io, _ := newIO("2026-05-05\n单天\n")
	s := SegmentEdit{Index: 1, Total: 1, Start: time.Now(), End: time.Now()}
	r, err := io.EditSegment(s)
	if err != nil {
		t.Fatalf("EditSegment: %v", err)
	}
	want := time.Date(2026, 5, 5, 0, 0, 0, 0, time.Local)
	if !r.Start.Equal(want) || !r.End.Equal(want) {
		t.Errorf("single date should produce start==end: %s..%s", r.Start, r.End)
	}
}

func TestAskTarget_EmptyReturnsDefault(t *testing.T) {
	io, _ := newIO("\n")
	got, err := io.AskTarget("/home/x/Backups")
	if err != nil || got != "/home/x/Backups" {
		t.Errorf("default target: got %q err %v", got, err)
	}
}

func TestAskTarget_OverridesWithUserInput(t *testing.T) {
	io, _ := newIO("/mnt/external/Archive\n")
	got, err := io.AskTarget("/home/x/Backups")
	if err != nil || got != "/mnt/external/Archive" {
		t.Errorf("override target: got %q err %v", got, err)
	}
}

func TestAskTarget_TrimsWhitespace(t *testing.T) {
	io, _ := newIO("   /tmp/out   \n")
	got, _ := io.AskTarget("/default")
	if got != "/tmp/out" {
		t.Errorf("trim target: got %q", got)
	}
}

// 用户从资源管理器/Finder 复制带引号的路径粘进来时，stdin 不会自动剥引号。
// 不剥的话 filepath.Abs 把首字符 " 当相对路径起点，路径会被错误地拼到 cwd 后。
func TestAskTarget_StripsPastedDoubleQuotes(t *testing.T) {
	io, _ := newIO("\"E:\\multimedia\"\n")
	got, err := io.AskTarget("C:\\Backups")
	if err != nil {
		t.Fatalf("AskTarget: %v", err)
	}
	if got != "E:\\multimedia" {
		t.Errorf("expected stripped path, got %q", got)
	}
}

func TestAskTarget_StripsPastedSingleQuotes(t *testing.T) {
	io, _ := newIO("'/Volumes/Photos'\n")
	got, _ := io.AskTarget("/default")
	if got != "/Volumes/Photos" {
		t.Errorf("expected stripped path, got %q", got)
	}
}

// 不成对的引号原样保留（用户有可能就想这么命名目录）。
func TestAskTarget_KeepsUnpairedQuote(t *testing.T) {
	io, _ := newIO("/tmp/weird\"name\n")
	got, _ := io.AskTarget("/default")
	if got != "/tmp/weird\"name" {
		t.Errorf("expected unpaired quote preserved, got %q", got)
	}
}

// 含空格的引号路径：剥掉外层引号，内部空格保留。
func TestAskTarget_StripsQuotesAroundPathWithSpaces(t *testing.T) {
	io, _ := newIO("\"/My Drive/2026 footage\"\n")
	got, _ := io.AskTarget("/default")
	if got != "/My Drive/2026 footage" {
		t.Errorf("expected spaces preserved inside stripped quotes, got %q", got)
	}
}

func TestConfirmProceed_AcceptsYAndEmpty(t *testing.T) {
	for _, in := range []string{"\n", "y\n", "yes\n", "Y\n"} {
		io, _ := newIO(in)
		ok, err := io.ConfirmProceed(ProceedSummary{Target: "/x", TotalFiles: 1})
		if err != nil || !ok {
			t.Errorf("input %q: got ok=%v err=%v", in, ok, err)
		}
	}
}

func TestConfirmProceed_RejectsN(t *testing.T) {
	io, _ := newIO("n\n")
	ok, err := io.ConfirmProceed(ProceedSummary{Target: "/x"})
	if err != nil || ok {
		t.Errorf("expected reject, got ok=%v err=%v", ok, err)
	}
}

func TestConfirmProceed_RendersSegments(t *testing.T) {
	io, out := newIO("y\n")
	_, _ = io.ConfirmProceed(ProceedSummary{
		Target: "/tmp/out", Device: "SONY ZVE10M2",
		TotalFiles: 4, TotalBytes: 1_300_000_000,
		Segments: []string{"a-b 五一假期前 (2 files)", "c-d 五一假期 (2 files)"},
	})
	s := out.String()
	for _, want := range []string{"4 个文件", "1.2 GB", "/tmp/out", "SONY ZVE10M2", "五一假期前", "五一假期"} {
		if !contains(s, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, s)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	}())
}

func TestConfirmOverwrite_RejectsByDefault(t *testing.T) {
	// 默认（回车 / "n"）应保留旧文件。
	for _, in := range []string{"\n", "n\n", "no\n", "N\n"} {
		io, _ := newIO(in)
		ok, err := io.ConfirmOverwrite(ConflictInfo{
			RelPath: "C0001.MP4", SrcSize: 100, DstSize: 100,
			SrcHash: "abc12345deadbeef", DstHash: "ff998877cafebabe",
		})
		if err != nil || ok {
			t.Errorf("input %q: expected default reject, got ok=%v err=%v", in, ok, err)
		}
	}
}

func TestConfirmOverwrite_AcceptsExplicitY(t *testing.T) {
	for _, in := range []string{"y\n", "Y\n", "yes\n"} {
		io, _ := newIO(in)
		ok, err := io.ConfirmOverwrite(ConflictInfo{
			RelPath: "C0001.MP4", SrcSize: 100, DstSize: 100,
			SrcHash: "abc12345", DstHash: "ff998877",
		})
		if err != nil || !ok {
			t.Errorf("input %q: expected overwrite accept, got ok=%v err=%v", in, ok, err)
		}
	}
}

func TestConfirmOverwrite_RendersHashesAndTrash(t *testing.T) {
	io, out := newIO("n\n")
	_, _ = io.ConfirmOverwrite(ConflictInfo{
		RelPath:   "PRIVATE/M4ROOT/CLIP/C0001.MP4",
		TrashPath: "/backups/.ingest-trash/2026-05-10T12-34-56/C0001.MP4",
		SrcSize:   1_300_000, DstSize: 1_300_000,
		SrcHash: "abc12345deadbeef", DstHash: "ff998877cafebabe",
	})
	s := out.String()
	for _, want := range []string{
		"PRIVATE/M4ROOT/CLIP/C0001.MP4",
		"abc12345",
		"ff998877",
		".ingest-trash/2026-05-10T12-34-56/C0001.MP4",
		"覆盖? [y/N]",
	} {
		if !contains(s, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, s)
		}
	}
}

// size 不同时应在目标行后标注「大小不同」，且不展示 hash（节省时间）。
func TestConfirmOverwrite_DifferentSizeNoHashes(t *testing.T) {
	io, out := newIO("n\n")
	_, _ = io.ConfirmOverwrite(ConflictInfo{
		RelPath: "C0001.MP4",
		SrcSize: 100, DstSize: 50,
	})
	s := out.String()
	if !contains(s, "大小不同") {
		t.Errorf("expected 大小不同 marker in output, got:\n%s", s)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:                "512 B",
		2048:               "2.0 KB",
		5_500_000:          "5.2 MB",
		4_200_000_000:      "3.9 GB",
		2_500_000_000_000:  "2.3 TB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
