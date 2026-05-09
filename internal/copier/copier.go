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
	// ResultConflict: dst 已存在但与 src 内容实质不同（size 不同 或 同 size 不同 hash）。
	// SafeCopy 不会静默覆盖，由调用方根据策略决定是覆盖（OverwriteCopy）还是跳过。
	ResultConflict
)

func (r Result) String() string {
	switch r {
	case ResultCopied:
		return "copied"
	case ResultSkipped:
		return "skipped"
	case ResultFailed:
		return "failed"
	case ResultConflict:
		return "conflict"
	}
	return "unknown"
}

type Outcome struct {
	Result Result
	Hash   string
	Bytes  int64
	Err    error

	// 仅在 ResultConflict 时有意义，避免上层为了 prompt 再算一次。
	SrcHash string
	DstHash string
	DstSize int64
}

// SafeCopy 实现 §FR-005 协议：流式 src → dst 旁路 tmp 文件，写入时同步算 src
// xxHash64，再从磁盘 re-hash tmp 校验，最后原子 rename。
//
// 三种 dst 已存在的情形：
//   - size 同 + hash 同（含 db 快路径） → ResultSkipped
//   - size 同 + hash 不同              → ResultConflict（不覆盖，留给调用方）
//   - size 不同                        → ResultConflict（不覆盖，留给调用方）
//
// 要强制覆盖请调用 OverwriteCopy。
func SafeCopy(src, dst, deviceID, volumeID string, store *db.DB) Outcome {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return Outcome{Result: ResultFailed, Err: fmt.Errorf("stat src: %w", err)}
	}
	srcSize := srcInfo.Size()

	if dstInfo, err := os.Stat(dst); err == nil {
		dstSize := dstInfo.Size()
		if dstSize == srcSize {
			if rec, _ := store.Get(dst); rec != nil && rec.Size == srcSize {
				if srcHash, hErr := hashFile(src); hErr == nil && srcHash == rec.Hash {
					return Outcome{Result: ResultSkipped, Hash: srcHash, Bytes: srcSize}
				}
			}
			srcHash, sErr := hashFile(src)
			dstHash, dErr := hashFile(dst)
			if sErr != nil {
				return Outcome{Result: ResultFailed, Err: fmt.Errorf("hash src: %w", sErr)}
			}
			if dErr != nil {
				return Outcome{Result: ResultFailed, Err: fmt.Errorf("hash dst: %w", dErr)}
			}
			if srcHash == dstHash {
				_ = store.Put(db.Record{
					SourcePath: src, TargetPath: dst, DeviceID: deviceID,
					VolumeID: volumeID, Size: srcSize, Hash: srcHash,
				})
				return Outcome{Result: ResultSkipped, Hash: srcHash, Bytes: srcSize}
			}
			return Outcome{
				Result: ResultConflict, Bytes: srcSize,
				SrcHash: srcHash, DstHash: dstHash, DstSize: dstSize,
			}
		}
		// size 不同：不算 hash（昂贵），直接 conflict；上层 prompt 时只展示 size 差。
		return Outcome{Result: ResultConflict, Bytes: srcSize, DstSize: dstSize}
	}

	return doCopy(src, dst, deviceID, volumeID, srcInfo, store)
}

// OverwriteCopy 与 SafeCopy 等价，但当 dst 已存在时一律覆盖：
//   - 若 trashDir 非空，先把旧 dst rename 到 trashDir/<dst 相对 trashDir 父目录的相对路径>，
//     失败则 fallback 到 unlink（保证向前推进）。
//   - 若 trashDir 为空，直接 unlink 旧 dst。
//
// 调用方必须在确认覆盖意图后再调（典型路径：SafeCopy 返回 Conflict → prompt y → OverwriteCopy）。
func OverwriteCopy(src, dst, deviceID, volumeID, trashDir string, store *db.DB) Outcome {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return Outcome{Result: ResultFailed, Err: fmt.Errorf("stat src: %w", err)}
	}

	if _, err := os.Stat(dst); err == nil {
		if err := moveToTrashOrUnlink(dst, trashDir); err != nil {
			return Outcome{Result: ResultFailed, Err: fmt.Errorf("clear existing dst: %w", err)}
		}
	}

	return doCopy(src, dst, deviceID, volumeID, srcInfo, store)
}

func doCopy(src, dst, deviceID, volumeID string, srcInfo os.FileInfo, store *db.DB) Outcome {
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

// moveToTrashOrUnlink 优先 rename 到 trashDir/<dst 文件名>；rename 失败（典型：
// trashDir 跨分区或不可达）时回退到 unlink。trashDir 为空直接 unlink。
//
// 注意：trashDir/<文件名> 在同一次 ingest 多个段都覆盖到同名文件时会冲突；
// 调用方应确保每次 ingest 的 trashDir 已经按 timestamp 分桶，且这里把
// dst 的相对结构带过去（取 dst 的 base + 一个唯一后缀防撞）。
func moveToTrashOrUnlink(dst, trashDir string) error {
	if trashDir == "" {
		return os.Remove(dst)
	}
	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		return os.Remove(dst)
	}
	trashPath := filepath.Join(trashDir, filepath.Base(dst))
	if _, err := os.Stat(trashPath); err == nil {
		trashPath = trashPath + "." + randomSuffix()
	}
	if err := os.Rename(dst, trashPath); err == nil {
		return nil
	}
	return os.Remove(dst)
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
