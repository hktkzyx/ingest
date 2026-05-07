//go:build linux

package mount

import (
	"strings"
	"testing"
)

func TestParseProcMounts_KeepsRemovablePathsAndDropsVirtual(t *testing.T) {
	input := strings.Join([]string{
		"/dev/nvme0n1p2 / ext4 rw,relatime 0 0",
		"proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0",
		"sysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0",
		"tmpfs /run tmpfs rw,nosuid,nodev 0 0",
		"/dev/sdb1 /media/brooks/SONY vfat rw,relatime 0 0",
		"/dev/sdc1 /run/media/brooks/DJI exfat rw,relatime 0 0",
		"/dev/sdd1 /mnt/extra-disk ext4 rw 0 0",
		"/dev/loop0 /snap/core22/1234 squashfs ro 0 0",
		"",
	}, "\n")

	got := parseProcMounts(strings.NewReader(input))
	want := map[string]string{
		"/media/brooks/SONY":      "vfat",
		"/run/media/brooks/DJI":   "exfat",
		"/mnt/extra-disk":         "ext4",
	}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d (%+v), want %d", len(got), got, len(want))
	}
	for _, v := range got {
		fs, ok := want[v.Path]
		if !ok {
			t.Errorf("unexpected path %q", v.Path)
			continue
		}
		if v.FSType != fs {
			t.Errorf("path %q: fstype got %q want %q", v.Path, v.FSType, fs)
		}
	}
}

func TestParseProcMounts_HandlesEscapedSpacesInLabel(t *testing.T) {
	input := "/dev/sdb1 /media/brooks/My\\040Card vfat rw 0 0\n"
	got := parseProcMounts(strings.NewReader(input))
	if len(got) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(got))
	}
	if got[0].Path != "/media/brooks/My Card" {
		t.Fatalf("unescape mismatch: %q", got[0].Path)
	}
	if got[0].Label != "My Card" {
		t.Fatalf("label mismatch: %q", got[0].Label)
	}
}

func TestParseProcMounts_DedupesDuplicatePaths(t *testing.T) {
	// 同一挂载点出现两次（autofs 触发后再 bind mount）；只算一个。
	input := "/dev/sdb1 /media/brooks/SD vfat rw 0 0\n/dev/sdb1 /media/brooks/SD vfat rw 0 0\n"
	got := parseProcMounts(strings.NewReader(input))
	if len(got) != 1 {
		t.Fatalf("expected 1 dedup volume, got %d", len(got))
	}
}

func TestUnescapeMount(t *testing.T) {
	cases := map[string]string{
		"/no/escape":         "/no/escape",
		"/with\\040space":    "/with space",
		"/tab\\011here":      "/tab\there",
		"/many\\040\\040sp":  "/many  sp",
	}
	for in, want := range cases {
		if got := unescapeMount(in); got != want {
			t.Errorf("unescapeMount(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHasRemovablePrefix(t *testing.T) {
	cases := map[string]bool{
		"/media/brooks/SD":     true,
		"/run/media/foo/SD":    true,
		"/mnt/disk":            true,
		"/media/":              false, // bare prefix, no suffix
		"/home/brooks":         false,
		"/":                    false,
	}
	for in, want := range cases {
		if got := hasRemovablePrefix(in); got != want {
			t.Errorf("hasRemovablePrefix(%q) = %v, want %v", in, got, want)
		}
	}
}
