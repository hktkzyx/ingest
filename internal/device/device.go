package device

import (
	"os"
	"path/filepath"
	"strings"
)

type Rule struct {
	ID           string
	Name         string
	Manufacturer string
	VolumeLabels []string
	Directories  []string
	FilePatterns []string
}

type Match struct {
	Rule       Rule
	Confidence float64
	Reason     string
}

var BuiltinRules = []Rule{
	{
		ID: "zve10m2", Name: "Sony ZV-E10M2", Manufacturer: "Sony",
		VolumeLabels: []string{"SONY"},
		Directories:  []string{"PRIVATE/SONY", "DCIM"},
		FilePatterns: []string{"C*.MP4", "C*.MTS", "DSC*.ARW", "DSC*.JPG"},
	},
	{
		ID: "pocket3", Name: "DJI Pocket3", Manufacturer: "DJI",
		VolumeLabels: []string{"DJI"},
		Directories:  []string{"DCIM/100MEDIA"},
		FilePatterns: []string{"DJI_*.MP4"},
	},
}

func Detect(volumePath, volumeLabel string) *Match {
	var best *Match
	for _, r := range BuiltinRules {
		m := score(r, volumePath, volumeLabel)
		if m == nil {
			continue
		}
		if best == nil || m.Confidence > best.Confidence {
			best = m
		}
	}
	return best
}

func score(r Rule, root, label string) *Match {
	upper := strings.ToUpper(label)
	for _, lab := range r.VolumeLabels {
		if upper != "" && strings.Contains(upper, strings.ToUpper(lab)) {
			return &Match{Rule: r, Confidence: 0.9, Reason: "volume label"}
		}
	}
	dirHits := 0
	for _, d := range r.Directories {
		if st, err := os.Stat(filepath.Join(root, d)); err == nil && st.IsDir() {
			dirHits++
		}
	}
	if len(r.Directories) > 0 && dirHits == len(r.Directories) {
		return &Match{Rule: r, Confidence: 0.8, Reason: "directory structure"}
	}
	if matchesAny(r.FilePatterns, root) {
		return &Match{Rule: r, Confidence: 0.7, Reason: "file pattern"}
	}
	if dirHits > 0 {
		return &Match{Rule: r, Confidence: 0.5 + 0.1*float64(dirHits), Reason: "partial directory"}
	}
	return nil
}

func matchesAny(patterns []string, root string) bool {
	for _, p := range patterns {
		for _, glob := range []string{
			filepath.Join(root, p),
			filepath.Join(root, "*", p),
			filepath.Join(root, "*", "*", p),
		} {
			if hits, _ := filepath.Glob(glob); len(hits) > 0 {
				return true
			}
		}
	}
	return false
}
