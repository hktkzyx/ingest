//go:build darwin

package mount

import (
	"fmt"
	"os"
	"path/filepath"
)

// macOS 把所有外接卷挂在 /Volumes 下，目录名即卷标。系统盘也会出现在这里
// （/Volumes/Macintosh HD），但那是个固定名字，加上一些惯例排除即可。
//
// 不依赖 diskutil/df：它们会启动子进程、解析 plist、容易踩到环境差异。
// 直接 readdir 是最稳定的做法，对 SD 卡这类典型用例完全够用。
const volumesDir = "/Volumes"

// systemDiskNames 是 macOS 用于本机硬盘的常见名字。出现这些名字的卷不会
// 被列为候选——即使技术上"可读"，把它当 SD 卡处理用户也不可能要这样。
var systemDiskNames = map[string]bool{
	"Macintosh HD":           true,
	"Macintosh HD - Data":    true,
	"Macintosh HD — Data":    true, // 长破折号变体（Apple 在某些 locale 用的）
}

// List 列出 /Volumes 下所有挂载点（除系统盘）。
func List() ([]Volume, error) {
	entries, err := os.ReadDir(volumesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", volumesDir, err)
	}
	var out []Volume
	for _, e := range entries {
		name := e.Name()
		if systemDiskNames[name] {
			continue
		}
		// /Volumes 下的条目都是 firmlink / mount，dir 与 symlink 都接受。
		if !e.IsDir() {
			info, err := e.Info()
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				continue
			}
		}
		out = append(out, Volume{
			Path:  filepath.Join(volumesDir, name),
			Label: name, // 目录名就是卷标
		})
	}
	return out, nil
}
