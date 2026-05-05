package copier

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cespare/xxhash/v2"

	"github.com/hktkzyx/ingest/internal/db"
)

type Result int

const (
	ResultCopied Result = iota
	ResultSkipped
	ResultFailed
)

func (r Result) String() string {
	switch r {
	case ResultCopied:
		return "copied"
	case ResultSkipped:
		return "skipped"
	case ResultFailed:
		return "failed"
	}
	return "unknown"
}

type Outcome struct {
	Result Result
	Hash   string
	Bytes  int64
	Err    error
}

// SafeCopy implements the §FR-005 protocol: stream src → temp file alongside
// dst with simultaneous xxHash64 over the source bytes, then re-hash the temp
// from disk to verify the write landed correctly, then atomically rename.
//
// Incremental: if dst already exists at the same size and the db has a
// matching record, we just confirm the source hash and skip. If there's no
// record (or it disagrees), we re-hash both ends to decide.
func SafeCopy(src, dst, deviceID, volumeID string, store *db.DB) Outcome {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return Outcome{Result: ResultFailed, Err: fmt.Errorf("stat src: %w", err)}
	}
	srcSize := srcInfo.Size()

	if dstInfo, err := os.Stat(dst); err == nil && dstInfo.Size() == srcSize {
		if rec, _ := store.Get(dst); rec != nil && rec.Size == srcSize {
			if srcHash, hErr := hashFile(src); hErr == nil && srcHash == rec.Hash {
				return Outcome{Result: ResultSkipped, Hash: srcHash, Bytes: srcSize}
			}
		}
		srcHash, sErr := hashFile(src)
		dstHash, dErr := hashFile(dst)
		if sErr == nil && dErr == nil && srcHash == dstHash {
			_ = store.Put(db.Record{
				SourcePath: src, TargetPath: dst, DeviceID: deviceID,
				VolumeID: volumeID, Size: srcSize, Hash: srcHash,
			})
			return Outcome{Result: ResultSkipped, Hash: srcHash, Bytes: srcSize}
		}
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Outcome{Result: ResultFailed, Err: fmt.Errorf("mkdir target: %w", err)}
	}

	tmp := dst + ".ingest.tmp." + randomSuffix()
	srcHash, dstHash, n, err := streamCopy(src, tmp)
	if err != nil {
		os.Remove(tmp)
		return Outcome{Result: ResultFailed, Err: err}
	}
	if srcHash != dstHash {
		os.Remove(tmp)
		return Outcome{Result: ResultFailed, Err: fmt.Errorf("checksum mismatch: src=%s dst=%s", srcHash, dstHash)}
	}

	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return Outcome{Result: ResultFailed, Err: fmt.Errorf("rename: %w", err)}
	}

	_ = os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime())
	_ = os.Chmod(dst, srcInfo.Mode().Perm())

	if err := store.Put(db.Record{
		SourcePath: src, TargetPath: dst, DeviceID: deviceID,
		VolumeID: volumeID, Size: n, Hash: srcHash,
	}); err != nil {
		return Outcome{Result: ResultCopied, Hash: srcHash, Bytes: n, Err: fmt.Errorf("db record: %w", err)}
	}
	return Outcome{Result: ResultCopied, Hash: srcHash, Bytes: n}
}

func streamCopy(src, dst string) (srcHash, dstHash string, n int64, err error) {
	in, err := os.Open(src)
	if err != nil {
		return "", "", 0, fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", "", 0, fmt.Errorf("create tmp: %w", err)
	}
	defer out.Close()

	h := xxhash.New()
	mw := io.MultiWriter(out, h)
	n, err = io.Copy(mw, in)
	if err != nil {
		return "", "", n, fmt.Errorf("copy: %w", err)
	}
	if err := out.Sync(); err != nil {
		return "", "", n, fmt.Errorf("sync: %w", err)
	}
	srcHash = hex.EncodeToString(h.Sum(nil))

	dstHash, err = hashFile(dst)
	if err != nil {
		return "", "", n, fmt.Errorf("verify: %w", err)
	}
	return srcHash, dstHash, n, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := xxhash.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func randomSuffix() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "rnd"
	}
	return hex.EncodeToString(b[:])
}
