// Package mount 列举当前系统上挂载的可移动卷。
//
// 各平台实现独立文件（mount_linux.go / mount_darwin.go / mount_windows.go），
// 由 build tag 选择编译。各平台只需返回 List 函数能用的 Volume 切片即可，
// 上层调用方（cmd/ingest）再按 device.Detect 过滤"看起来像 SD 卡"的候选。
//
// 这个包是尽力而为，无法穷尽所有挂载工具的奇怪输出（autofs、bind mount、
// 内核虚拟文件系统）。各平台实现里都白名单了"看起来像可移动介质挂载点"
// 的路径前缀；新挂载点出现时再扩。
package mount

// Volume 描述一个已挂载的卷。
type Volume struct {
	Path   string // 挂载点绝对路径，例如 /Volumes/SONY_XYZ 或 /media/brooks/SD
	Label  string // 卷标；取不到时退化为 filepath.Base(Path)
	FSType string // 文件系统类型；取不到时为空字符串
}
