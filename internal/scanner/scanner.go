package scanner

import (
	"io/fs"
	"path/filepath"
	"strings"
)

var MediaExtensions = map[string]bool{
	".mp4": true, ".mov": true, ".m4v": true,
	".avi": true, ".mkv": true, ".mts": true, ".m2ts": true,
	".jpg": true, ".jpeg": true,
	".arw": true, ".raw": true, ".cr2": true, ".cr3": true,
	".nef": true, ".dng": true, ".heic": true, ".png": true,
	".wav": true, ".mp3": true, ".aac": true, ".bwf": true,
}

var SidecarExtensions = map[string]bool{
	".thm": true, ".xml": true, ".srt": true, ".lrc": true,
}

type File struct {
	Path    string
	RelPath string
	Size    int64
	Info    fs.FileInfo
	IsMedia bool
}

func IsMedia(name string) bool {
	return MediaExtensions[strings.ToLower(filepath.Ext(name))]
}

func IsSidecar(name string) bool {
	return SidecarExtensions[strings.ToLower(filepath.Ext(name))]
}

func Scan(root string) ([]File, error) {
	var out []File
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		isMedia := IsMedia(d.Name())
		if !isMedia && !IsSidecar(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, File{
			Path:    path,
			RelPath: rel,
			Size:    info.Size(),
			Info:    info,
			IsMedia: isMedia,
		})
		return nil
	})
	return out, err
}
