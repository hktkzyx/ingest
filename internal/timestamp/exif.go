package timestamp

import (
	"fmt"
	"time"

	exif "github.com/dsoprea/go-exif/v3"
)

// extractEXIF 读取 JPEG / TIFF / 多数 RAW（含 ARW/NEF/CR2/DNG）的 EXIF
// DateTimeOriginal 字段。该字段无时区信息，按本地时区解释。
func extractEXIF(path string) (time.Time, error) {
	raw, err := exif.SearchFileAndExtractExif(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("read exif: %w", err)
	}
	entries, _, err := exif.GetFlatExifData(raw, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse exif: %w", err)
	}
	for _, want := range []string{"DateTimeOriginal", "DateTimeDigitized", "DateTime"} {
		for _, e := range entries {
			if e.TagName != want {
				continue
			}
			s, ok := e.Value.(string)
			if !ok {
				continue
			}
			t, perr := time.ParseInLocation("2006:01:02 15:04:05", s, time.Local)
			if perr == nil {
				return t, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("no DateTime* tag in exif")
}
