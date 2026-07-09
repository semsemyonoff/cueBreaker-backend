package scan

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// coverNamePrefixes lists preferred cover-file stems, in priority order.
var coverNamePrefixes = []string{"cover", "folder", "front", "album"}

var coverImageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".bmp":  true,
	".gif":  true,
	".webp": true,
}

// FindCover looks for a cover image directly inside dir: a file whose stem
// (case-insensitive) is "cover", "folder", "front" or "album" and whose
// extension is a known image type wins, in that priority order; failing
// that, the alphabetically first image file in dir is used. The match is
// rejected unless its real path (after resolving symlinks) stays under
// dir's real path, blocking symlink-escape covers.
func FindCover(dir string) (path string, ok bool) {
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", false
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}

	var images []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if coverImageExts[strings.ToLower(filepath.Ext(name))] {
			images = append(images, name)
		}
	}
	sort.Strings(images)

	for _, prefix := range coverNamePrefixes {
		for _, name := range images {
			stem := strings.TrimSuffix(name, filepath.Ext(name))
			if !strings.EqualFold(stem, prefix) {
				continue
			}
			if p, ok := containedCoverFile(dir, realDir, name); ok {
				return p, true
			}
		}
	}

	for _, name := range images {
		if p, ok := containedCoverFile(dir, realDir, name); ok {
			return p, true
		}
	}

	return "", false
}

// containedCoverFile joins dir and name, and returns that path only if it
// names an existing file whose real path (after resolving symlinks) stays
// under realDir.
func containedCoverFile(dir, realDir, name string) (string, bool) {
	candidate := filepath.Join(dir, name)

	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() {
		return "", false
	}

	realCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false
	}

	rel, err := filepath.Rel(realDir, realCandidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}

	return candidate, true
}
