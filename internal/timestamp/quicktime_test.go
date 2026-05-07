package timestamp

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 构造一个最小有效的 MP4：ftyp + moov{mvhd v0}。
// mvhd v0 payload：1B version(0) + 3B flags + 4B creation_time + 4B modification_time
// + 4B time_scale + 4B duration + 余下 80B 默认值，共 100B。
func buildMinimalMP4(creationSecondsSince1904 uint32) []byte {
	atom := func(typ string, body []byte) []byte {
		var buf bytes.Buffer
		_ = binary.Write(&buf, binary.BigEndian, uint32(8+len(body)))
		buf.WriteString(typ)
		buf.Write(body)
		return buf.Bytes()
	}

	mvhd := make([]byte, 100)
	// version=0, flags=0
	binary.BigEndian.PutUint32(mvhd[4:8], creationSecondsSince1904) // creation_time
	binary.BigEndian.PutUint32(mvhd[8:12], creationSecondsSince1904) // modification_time
	binary.BigEndian.PutUint32(mvhd[12:16], 1000)                    // time_scale
	binary.BigEndian.PutUint32(mvhd[16:20], 0)                       // duration

	moov := atom("moov", atom("mvhd", mvhd))
	ftyp := atom("ftyp", []byte("isom\x00\x00\x02\x00isomiso2"))
	return append(ftyp, moov...)
}

func TestExtractQuickTime_MinimalMP4(t *testing.T) {
	want := time.Date(2026, 4, 27, 9, 30, 0, 0, time.UTC)
	secs := uint32(want.Sub(qtEpoch).Seconds())

	tmp := filepath.Join(t.TempDir(), "test.mp4")
	if err := os.WriteFile(tmp, buildMinimalMP4(secs), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := extractQuickTime(tmp)
	if err != nil {
		t.Fatalf("extractQuickTime: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("creation_time mismatch: got %s want %s", got, want)
	}
}

func TestExtractQuickTime_MoovAfterFtypAndMdat(t *testing.T) {
	// 模拟非 faststart 文件：ftyp + mdat + moov 顺序。findAtom 应跳过 mdat 找到 moov。
	want := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	secs := uint32(want.Sub(qtEpoch).Seconds())

	atom := func(typ string, body []byte) []byte {
		var buf bytes.Buffer
		_ = binary.Write(&buf, binary.BigEndian, uint32(8+len(body)))
		buf.WriteString(typ)
		buf.Write(body)
		return buf.Bytes()
	}

	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[4:8], secs)
	binary.BigEndian.PutUint32(mvhd[8:12], secs)
	binary.BigEndian.PutUint32(mvhd[12:16], 1000)

	mdat := atom("mdat", make([]byte, 4096)) // 假 payload
	moov := atom("moov", atom("mvhd", mvhd))
	ftyp := atom("ftyp", []byte("isom\x00\x00\x02\x00isomiso2"))

	tmp := filepath.Join(t.TempDir(), "test.mp4")
	if err := os.WriteFile(tmp, append(append(ftyp, mdat...), moov...), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := extractQuickTime(tmp)
	if err != nil {
		t.Fatalf("extractQuickTime: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("creation_time mismatch: got %s want %s", got, want)
	}
}

func TestExtractQuickTime_NotMP4(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "not-mp4.mp4")
	if err := os.WriteFile(tmp, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := extractQuickTime(tmp)
	if err == nil {
		t.Fatal("expected error on non-mp4 input, got nil")
	}
}
