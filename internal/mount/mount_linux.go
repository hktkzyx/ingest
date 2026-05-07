//go:build linux

package mount

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// removablePrefixes 是 Linux 桌面环境通常挂载可移动介质的位置：
//   - /media/<user>/<label>      ← Ubuntu/Debian + udisks2
//   - /run/media/<user>/<label>  ← Fedora/Arch + udisks2
//   - /mnt/<anything>            ← 手动挂载惯例
//
// 我们故意不收 / 和 /home 下的挂载（避免把根分区或加密 home 当成 SD 卡）。
// 调用方再用 device.Detect 二次筛"看起来像相机存储"的卷。
var removablePrefixes = []string{
	"/media/",
	"/run/media/",
	"/mnt/",
}

// 这些 fstype 是内核虚拟文件系统或我们绝不可能要扫的，遇到直接跳过。
var skipFSType = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "proc": true, "sysfs": true,
	"cgroup": true, "cgroup2": true, "devpts": true, "mqueue": true,
	"securityfs": true, "pstore": true, "bpf": true, "tracefs": true,
	"debugfs": true, "configfs": true, "fusectl": true, "rpc_pipefs": true,
	"hugetlbfs": true, "binfmt_misc": true, "autofs": true, "nsfs": true,
	"squashfs": true, "overlay": true,
}

// List 解析 /proc/mounts 找出可移动介质挂载点。
func List() ([]Volume, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, fmt.Errorf("open /proc/mounts: %w", err)
	}
	defer f.Close()
	return parseProcMounts(f), nil
}

// parseProcMounts 是 List 的可测试核心。/proc/mounts 每行格式（man 5 fstab）：
//
//	<device> <mountpoint> <fstype> <options> <dump> <pass>
//
// 字段以空格分隔；含空格的字段会用八进制 \040 转义。
func parseProcMounts(r io.Reader) []Volume {
	var out []Volume
	seen := map[string]bool{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		mountpoint := unescapeMount(fields[1])
		fstype := fields[2]
		if skipFSType[fstype] {
			continue
		}
		if !hasRemovablePrefix(mountpoint) {
			continue
		}
		if seen[mountpoint] {
			continue
		}
		seen[mountpoint] = true
		out = append(out, Volume{
			Path:   mountpoint,
			Label:  filepath.Base(mountpoint), // udisks2 用卷标做目录名，所以 base 就是 label
			FSType: fstype,
		})
	}
	return out
}

func hasRemovablePrefix(p string) bool {
	for _, prefix := range removablePrefixes {
		if strings.HasPrefix(p, prefix) && len(p) > len(prefix) {
			return true
		}
	}
	return false
}

// unescapeMount 处理 /proc/mounts 的八进制转义（\040 = ' '、\011 = TAB、
// \012 = LF、\134 = '\'）。其它平台路径不会出现这些字符，但用户家目录里
// 含空格的卷标确实存在。
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			c := (s[i+1]-'0')*64 + (s[i+2]-'0')*8 + (s[i+3] - '0')
			b.WriteByte(c)
			i += 3
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
