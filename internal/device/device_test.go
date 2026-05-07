package device

import (
	"os"
	"path/filepath"
	"testing"
)

// makeRule 是 ZVE10M2 默认规则的精简版，便于测试。
func makeRule() Rule {
	return Rule{
		ID:           "zve10m2",
		Name:         "SONY ZVE10M2",
		Manufacturer: "SONY",
		VolumeLabels: []string{"SONY"},
		Directories:  []string{"PRIVATE/M4ROOT", "DCIM/100MSDCF"},
		FilePatterns: []string{"C*.MP4", "DSC*.JPG"},
	}
}

func mkdirs(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScore_VolumeLabelHitsHighest(t *testing.T) {
	root := t.TempDir()
	// 即使没有目录结构，卷标命中就应该 0.90。
	m := score(makeRule(), root, "SONY")
	if m == nil || m.Confidence != 0.9 || m.Reason != "volume label" {
		t.Fatalf("expected volume label match 0.90, got %+v", m)
	}
}

func TestScore_AllDirectoriesPresentGives080(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "PRIVATE/M4ROOT", "DCIM/100MSDCF")
	m := score(makeRule(), root, "")
	if m == nil || m.Confidence != 0.8 || m.Reason != "directory structure" {
		t.Fatalf("expected full-dir match 0.80, got %+v", m)
	}
}

func TestScore_FilePatternFallback(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "subdir")
	if err := os.WriteFile(filepath.Join(root, "subdir", "C0001.MP4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := score(makeRule(), root, "")
	if m == nil || m.Confidence != 0.7 || m.Reason != "file pattern" {
		t.Fatalf("expected file-pattern match 0.70, got %+v", m)
	}
}

// 关键回归：仅命中 1 个目录（杂牌 U 盘上有 DCIM/100MSDCF）不再触发 partial。
// 这是用户 v0.1.0-alpha.1 时遇到的 G:\ 误判（confidence 0.60）的修复。
func TestScore_SingleDirectoryHitDoesNotTriggerPartial(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "DCIM/100MSDCF") // 只命中 2 个目录中的 1 个
	m := score(makeRule(), root, "")
	if m != nil {
		t.Fatalf("single-dir hit should not match anymore, got %+v", m)
	}
}

func TestScore_TwoDirectoryHitsTriggerPartial(t *testing.T) {
	rule := Rule{
		ID:          "x",
		Directories: []string{"a", "b", "c"}, // 配 3 个，只命中 2 个
	}
	root := t.TempDir()
	mkdirs(t, root, "a", "b")
	m := score(rule, root, "")
	if m == nil || m.Reason != "partial directory" {
		t.Fatalf("expected partial directory match, got %+v", m)
	}
	want := 0.5 + 0.1*2
	if m.Confidence != want {
		t.Errorf("confidence = %.2f, want %.2f", m.Confidence, want)
	}
}

func TestScore_NoSignalReturnsNil(t *testing.T) {
	root := t.TempDir() // 完全空目录
	if m := score(makeRule(), root, ""); m != nil {
		t.Errorf("empty source should not match, got %+v", m)
	}
}

// 验证更新后的 Pocket3 规则能命中 DJI 实际卡布局：DCIM/DJI_001 + MISC/IDX + MISC/THM。
// 关键是 DJI_NNN 序号在 directories 里硬编码不可靠，所以用 MISC/IDX + MISC/THM。
func TestScore_Pocket3RealStructure(t *testing.T) {
	rule := Rule{
		ID:           "pocket3",
		Directories:  []string{"MISC/THM", "MISC/IDX"},
		FilePatterns: []string{"DJI_*.MP4"},
	}
	root := t.TempDir()
	mkdirs(t, root, "DCIM/DJI_001", "MISC/IDX", "MISC/THM/DJI_001")
	if err := os.WriteFile(filepath.Join(root, "DCIM", "DJI_001", "DJI_0001.MP4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := score(rule, root, "")
	if m == nil || m.Reason != "directory structure" || m.Confidence != 0.8 {
		t.Fatalf("Pocket3 should hit directory structure 0.80, got %+v", m)
	}
}

func TestDetect_PicksHighestConfidence(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "PRIVATE/M4ROOT", "DCIM/100MSDCF") // ZVE10M2 全中

	rules := []Rule{
		// "weak" 命中 PRIVATE/M4ROOT + DCIM 但少一个 EXTRA → partial 0.70
		{ID: "weak", Directories: []string{"PRIVATE/M4ROOT", "DCIM", "EXTRA"}},
		makeRule(), // 三档之最 0.80
	}
	m := Detect(rules, root, "")
	if m == nil || m.Rule.ID != "zve10m2" {
		t.Fatalf("expected zve10m2, got %+v", m)
	}
}
