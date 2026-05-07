package timestamp

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"
)

// QuickTime / ISO BMFF "movie time" 起点：1904-01-01 00:00:00 UTC。
var qtEpoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)

// extractQuickTime 解析 QuickTime / MP4 容器顶层 atom，定位 moov/mvhd
// 并读取 creation_time。该时间在容器规范里是 UTC。
//
// Atom 头：4 字节 BE size + 4 字节 type；size==1 时随后 8 字节 64-bit size；
// size==0 时扩展到文件末尾。多数文件 moov 在头部（faststart），少数（DJI 录制
// 中断的文件、非优化 MOV）在 mdat 之后——本实现不假设位置，扫到 moov 为止。
func extractQuickTime(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return time.Time{}, err
	}
	fileSize := st.Size()

	moovBody, err := findAtom(f, fileSize, 0, fileSize, "moov")
	if err != nil {
		return time.Time{}, fmt.Errorf("find moov: %w", err)
	}
	mvhdBody, err := findAtom(f, fileSize, moovBody.start, moovBody.end, "mvhd")
	if err != nil {
		return time.Time{}, fmt.Errorf("find mvhd: %w", err)
	}

	// mvhd payload：1 字节 version + 3 字节 flags + creation_time。
	// version 0：creation_time 4 字节；version 1：8 字节。
	if _, err := f.Seek(mvhdBody.start, io.SeekStart); err != nil {
		return time.Time{}, err
	}
	var head [4]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return time.Time{}, err
	}
	var secs uint64
	switch head[0] {
	case 0:
		var v uint32
		if err := binary.Read(f, binary.BigEndian, &v); err != nil {
			return time.Time{}, err
		}
		secs = uint64(v)
	case 1:
		if err := binary.Read(f, binary.BigEndian, &secs); err != nil {
			return time.Time{}, err
		}
	default:
		return time.Time{}, fmt.Errorf("mvhd unknown version %d", head[0])
	}
	if secs == 0 {
		return time.Time{}, fmt.Errorf("mvhd creation_time is zero")
	}
	return qtEpoch.Add(time.Duration(secs) * time.Second), nil
}

type atomRange struct {
	start int64 // body 起点（不含 8 或 16 字节头）
	end   int64 // body 结束（exclusive）
}

// findAtom 在 [scanStart, scanEnd) 区间内逐个读 atom 头，返回首个 type 匹配的 body 区间。
// 不递归：调用方控制是否再在子 atom 范围内搜下一层。
func findAtom(f *os.File, fileSize int64, scanStart, scanEnd int64, want string) (atomRange, error) {
	pos := scanStart
	for pos+8 <= scanEnd {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return atomRange{}, err
		}
		var hdr [8]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			return atomRange{}, err
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		typ := string(hdr[4:8])
		bodyStart := pos + 8
		var bodyEnd int64
		switch {
		case size == 0:
			bodyEnd = scanEnd
		case size == 1:
			var ext64 uint64
			if err := binary.Read(f, binary.BigEndian, &ext64); err != nil {
				return atomRange{}, err
			}
			bodyStart = pos + 16
			bodyEnd = pos + int64(ext64)
		default:
			bodyEnd = pos + size
		}
		if bodyEnd > scanEnd || bodyEnd <= bodyStart {
			return atomRange{}, fmt.Errorf("atom %q size out of range at %d", typ, pos)
		}
		if typ == want {
			return atomRange{start: bodyStart, end: bodyEnd}, nil
		}
		pos = bodyEnd
	}
	return atomRange{}, fmt.Errorf("atom %q not found", want)
}
