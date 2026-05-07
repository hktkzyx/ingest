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
	if _, err := io.EditSegment(s); err == nil {
		t.Errorf("expected error for empty name")
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
