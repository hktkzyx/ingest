//go:build windows

package mount

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Windows 上"可移动介质"由 GetDriveType 直接告诉我们：DRIVE_REMOVABLE 涵盖
// SD 卡、U 盘、读卡器映射出的盘符。我们不收 DRIVE_FIXED（机械/SSD）、
// DRIVE_CDROM（光驱）、DRIVE_REMOTE（网络共享）。
//
// 不引入 cgo：x/sys/windows 是纯 Go 的 syscall 封装，跨编译不需要额外 toolchain。
func List() ([]Volume, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("GetLogicalDrives: %w", err)
	}
	var out []Volume
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := fmt.Sprintf("%c:\\", 'A'+i)
		rootU16, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		if windows.GetDriveType(rootU16) != windows.DRIVE_REMOVABLE {
			continue
		}
		label, fstype := volumeInfo(rootU16)
		if label == "" {
			label = root // 没有卷标时退回盘符
		}
		out = append(out, Volume{
			Path:   root,
			Label:  label,
			FSType: fstype,
		})
	}
	return out, nil
}

// volumeInfo 调用 GetVolumeInformation 拿卷标和文件系统类型。
// 失败返回空字符串（典型场景是读卡器没插卡，盘符存在但卷不可访问）。
func volumeInfo(root *uint16) (label, fstype string) {
	var (
		volBuf  [windows.MAX_PATH + 1]uint16
		fsBuf   [windows.MAX_PATH + 1]uint16
		serial  uint32
		maxLen  uint32
		flags   uint32
	)
	err := windows.GetVolumeInformation(
		root,
		&volBuf[0], uint32(len(volBuf)),
		&serial, &maxLen, &flags,
		&fsBuf[0], uint32(len(fsBuf)),
	)
	if err != nil {
		return "", ""
	}
	return windows.UTF16ToString(volBuf[:]), windows.UTF16ToString(fsBuf[:])
}
