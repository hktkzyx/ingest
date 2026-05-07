// Package timestamp 从媒体文件本身提取拍摄时间。
// 失败时返回错误，调用方负责回退到 fs.FileInfo.ModTime()。
package timestamp

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Extract 按扩展名分发到 EXIF（图片）或 QuickTime atom（视频）解析器。
// 不支持的格式返回 ErrUnsupported；解析失败返回具体错误。
func Extract(path string) (time.Time, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".arw", ".cr2", ".cr3", ".nef", ".dng", ".heic", ".tiff", ".tif":
		return extractEXIF(path)
	case ".mp4", ".mov", ".m4v", ".mts", ".m2ts":
		return extractQuickTime(path)
	}
	return time.Time{}, fmt.Errorf("%w: %s", ErrUnsupported, ext)
}

var ErrUnsupported = fmt.Errorf("unsupported extension")
