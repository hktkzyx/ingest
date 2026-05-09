package copier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hktkzyx/ingest/internal/db"
)

func mkSrc(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func openDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "ingest.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// 基线：dst 不存在 → 拷贝。
func TestSafeCopy_FreshCopy(t *testing.T) {
	srcDir, dstDir := t.TempDir(), t.TempDir()
	src := mkSrc(t, srcDir, "a.mp4", "hello world")
	dst := filepath.Join(dstDir, "a.mp4")
	store := openDB(t)

	out := SafeCopy(src, dst, "dev1", "vol1", store)
	if out.Result != ResultCopied {
		t.Fatalf("expected ResultCopied, got %v err=%v", out.Result, out.Err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "hello world" {
		t.Errorf("dst content mismatch: %q", got)
	}
	rec, _ := store.Get(dst)
	if rec == nil || rec.Hash != out.Hash {
		t.Errorf("db record missing or hash mismatch: %+v", rec)
	}
}

// dst 存在且实质相同 → ResultSkipped，文件不被替换。
func TestSafeCopy_SkipsWhenContentIdentical(t *testing.T) {
	srcDir, dstDir := t.TempDir(), t.TempDir()
	src := mkSrc(t, srcDir, "a.mp4", "same content")
	dst := mkSrc(t, dstDir, "a.mp4", "same content")
	store := openDB(t)

	infoBefore, _ := os.Stat(dst)
	out := SafeCopy(src, dst, "dev1", "vol1", store)
	if out.Result != ResultSkipped {
		t.Fatalf("expected ResultSkipped, got %v err=%v", out.Result, out.Err)
	}
	infoAfter, _ := os.Stat(dst)
	if !os.SameFile(infoBefore, infoAfter) {
		t.Errorf("dst should not be rewritten when contents are identical")
	}
	rec, _ := store.Get(dst)
	if rec == nil || rec.Hash != out.Hash {
		t.Errorf("db record missing after skip: %+v", rec)
	}
}

// db 快路径：第二次运行同 src+dst 应直接命中 db skip。
func TestSafeCopy_SkipsViaDBFastPath(t *testing.T) {
	srcDir, dstDir := t.TempDir(), t.TempDir()
	src := mkSrc(t, srcDir, "a.mp4", "v1 v1 v1")
	dst := filepath.Join(dstDir, "a.mp4")
	store := openDB(t)

	if out := SafeCopy(src, dst, "d", "v", store); out.Result != ResultCopied {
		t.Fatalf("first run should copy: %+v", out)
	}
	if out := SafeCopy(src, dst, "d", "v", store); out.Result != ResultSkipped {
		t.Fatalf("second run should skip via db, got %+v", out)
	}
}

// 关键回归：dst 已存在 + size 同 + hash 不同 → ResultConflict（不再静默覆盖）。
// dst 内容必须保留原样，等待上层决策。
func TestSafeCopy_ConflictWhenSameSizeDifferentHash(t *testing.T) {
	srcDir, dstDir := t.TempDir(), t.TempDir()
	src := mkSrc(t, srcDir, "a.mp4", "AAAAAA")
	dst := mkSrc(t, dstDir, "a.mp4", "BBBBBB")
	store := openDB(t)

	out := SafeCopy(src, dst, "dev1", "vol1", store)
	if out.Result != ResultConflict {
		t.Fatalf("expected ResultConflict, got %v err=%v", out.Result, out.Err)
	}
	if out.SrcHash == "" || out.DstHash == "" || out.SrcHash == out.DstHash {
		t.Errorf("expected distinct src/dst hashes in conflict outcome: %+v", out)
	}
	if out.DstSize != int64(len("BBBBBB")) {
		t.Errorf("expected DstSize=6, got %d", out.DstSize)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "BBBBBB" {
		t.Errorf("dst must NOT be overwritten on conflict, got %q", got)
	}
}

// dst 已存在 + size 不同 → ResultConflict，dst 保留。
// size 不同时不算 hash（昂贵），所以 SrcHash/DstHash 应为空。
func TestSafeCopy_ConflictWhenDifferentSize(t *testing.T) {
	srcDir, dstDir := t.TempDir(), t.TempDir()
	src := mkSrc(t, srcDir, "a.mp4", "longer content here")
	dst := mkSrc(t, dstDir, "a.mp4", "short")
	store := openDB(t)

	out := SafeCopy(src, dst, "dev1", "vol1", store)
	if out.Result != ResultConflict {
		t.Fatalf("expected ResultConflict, got %v err=%v", out.Result, out.Err)
	}
	if out.DstSize != 5 {
		t.Errorf("expected DstSize=5, got %d", out.DstSize)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "short" {
		t.Errorf("dst must NOT be overwritten on conflict, got %q", got)
	}
}

// Q2：拷贝 → 删除目标 → 再次运行能否再拷贝。
func TestSafeCopy_RecopyAfterTargetDeleted(t *testing.T) {
	srcDir, dstDir := t.TempDir(), t.TempDir()
	src := mkSrc(t, srcDir, "a.mp4", "payload")
	dst := filepath.Join(dstDir, "a.mp4")
	store := openDB(t)

	if out := SafeCopy(src, dst, "d", "v", store); out.Result != ResultCopied {
		t.Fatalf("first copy failed: %+v", out)
	}
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	out := SafeCopy(src, dst, "d", "v", store)
	if out.Result != ResultCopied {
		t.Fatalf("recopy should have ResultCopied, got %v err=%v", out.Result, out.Err)
	}
	rec, _ := store.Get(dst)
	if rec == nil || rec.Hash != out.Hash {
		t.Errorf("db record should be updated after recopy: %+v", rec)
	}
}

// 综合：copy → 删除 → 再 copy → 再 skip。
func TestSafeCopy_DeleteRecopyThenSkip(t *testing.T) {
	srcDir, dstDir := t.TempDir(), t.TempDir()
	src := mkSrc(t, srcDir, "a.mp4", "abc def ghi")
	dst := filepath.Join(dstDir, "a.mp4")
	store := openDB(t)

	if out := SafeCopy(src, dst, "d", "v", store); out.Result != ResultCopied {
		t.Fatalf("step1: %+v", out)
	}
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	if out := SafeCopy(src, dst, "d", "v", store); out.Result != ResultCopied {
		t.Fatalf("step2 (recopy): %+v", out)
	}
	if out := SafeCopy(src, dst, "d", "v", store); out.Result != ResultSkipped {
		t.Fatalf("step3 (skip): %+v", out)
	}
}

// OverwriteCopy 把旧 dst 移到 trashDir 并写入新内容。
func TestOverwriteCopy_MovesOldToTrash(t *testing.T) {
	srcDir, dstDir, trashDir := t.TempDir(), t.TempDir(), t.TempDir()
	src := mkSrc(t, srcDir, "a.mp4", "NEW content")
	dst := mkSrc(t, dstDir, "a.mp4", "OLD content")
	store := openDB(t)

	out := OverwriteCopy(src, dst, "d", "v", trashDir, store)
	if out.Result != ResultCopied {
		t.Fatalf("expected ResultCopied, got %v err=%v", out.Result, out.Err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "NEW content" {
		t.Errorf("dst should be NEW content, got %q", got)
	}
	// 旧 dst 应在 trashDir 下，文件名不变（base name 即可）。
	trashed, _ := os.ReadFile(filepath.Join(trashDir, "a.mp4"))
	if string(trashed) != "OLD content" {
		t.Errorf("expected old content in trash, got %q", trashed)
	}
}

// trashDir 为空 → 旧 dst 直接 unlink，不留备份。
func TestOverwriteCopy_NoTrashDirUnlinksOld(t *testing.T) {
	srcDir, dstDir := t.TempDir(), t.TempDir()
	src := mkSrc(t, srcDir, "a.mp4", "NEW")
	dst := mkSrc(t, dstDir, "a.mp4", "OLD")
	store := openDB(t)

	out := OverwriteCopy(src, dst, "d", "v", "", store)
	if out.Result != ResultCopied {
		t.Fatalf("expected ResultCopied, got %+v", out)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "NEW" {
		t.Errorf("dst should be NEW, got %q", got)
	}
}

// 同名 trash 文件已存在时（典型：同一次 ingest 多段都冲突）→ 加唯一后缀防撞。
func TestOverwriteCopy_TrashCollisionGetsSuffix(t *testing.T) {
	srcDir, dstDir, trashDir := t.TempDir(), t.TempDir(), t.TempDir()
	// 先种一个同名 trash 文件占位。
	mkSrc(t, trashDir, "a.mp4", "earlier trash entry")

	src := mkSrc(t, srcDir, "a.mp4", "NEW")
	dst := mkSrc(t, dstDir, "a.mp4", "OLD")
	store := openDB(t)

	if out := OverwriteCopy(src, dst, "d", "v", trashDir, store); out.Result != ResultCopied {
		t.Fatalf("overwrite failed: %+v", out)
	}

	entries, _ := os.ReadDir(trashDir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 trash entries (placeholder + new), got %d: %v", len(entries), entries)
	}
}

// dst 不存在时 OverwriteCopy 行为应等价于 SafeCopy（fresh copy）。
func TestOverwriteCopy_FreshCopy(t *testing.T) {
	srcDir, dstDir, trashDir := t.TempDir(), t.TempDir(), t.TempDir()
	src := mkSrc(t, srcDir, "a.mp4", "fresh")
	dst := filepath.Join(dstDir, "a.mp4")
	store := openDB(t)

	out := OverwriteCopy(src, dst, "d", "v", trashDir, store)
	if out.Result != ResultCopied {
		t.Fatalf("expected ResultCopied, got %+v", out)
	}
	if entries, _ := os.ReadDir(trashDir); len(entries) != 0 {
		t.Errorf("trashDir should remain empty when dst was absent, got %d entries", len(entries))
	}
}
