package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"git.horn/cueBreaker/backend/internal/cue"
)

// Pair describes a directory holding a CUE sheet that references an
// existing, unsplit source FLAC/WAV, plus its already-split status against
// the output directory.
type Pair struct {
	Path         string   `json:"path"`
	AbsPath      string   `json:"abs_path"`
	CueFiles     []string `json:"cue_files"`
	FlacFiles    []string `json:"flac_files"`
	SplitDone    bool     `json:"split_done"`
	OutputTracks int      `json:"output_tracks"`
}

// FindPairs walks inputDir and returns every directory containing at least
// one CUE sheet that references an existing single source FLAC/WAV (i.e. an
// unsplit album), sorted by relative path. A directory whose CUE sheets are
// all multi-file (already split) or reference a missing source is skipped.
func FindPairs(inputDir, outputDir string) ([]Pair, error) {
	// Initialize non-nil so an empty library marshals to JSON [] rather than
	// null; the SPA treats the response as an array (items.length) and would
	// otherwise crash on the first-run empty scan.
	results := []Pair{}

	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A single unreadable entry (odd perms, transient mount, lost+found)
			// shouldn't blank the whole library; skip it and keep walking. Only
			// an unreadable input root is fatal.
			if path == inputDir {
				return err
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}

		var cueFiles, flacFiles []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			lower := strings.ToLower(name)
			switch {
			case strings.HasSuffix(lower, ".cue"):
				cueFiles = append(cueFiles, name)
			case strings.HasSuffix(lower, ".flac"):
				flacFiles = append(flacFiles, name)
			}
		}
		if len(cueFiles) == 0 {
			return nil
		}

		var validCues []string
		for _, cf := range cueFiles {
			if cue.HasSourceFLAC(filepath.Join(path, cf), path) {
				validCues = append(validCues, cf)
			}
		}
		if len(validCues) == 0 {
			return nil
		}
		sort.Strings(validCues)
		sort.Strings(flacFiles)

		relPath, err := filepath.Rel(inputDir, path)
		if err != nil {
			return err
		}

		firstCue := filepath.Join(path, validCues[0])
		done, outputTracks := CheckOutputStatus(outputDir, relPath, firstCue)

		results = append(results, Pair{
			Path:         relPath,
			AbsPath:      path,
			CueFiles:     validCues,
			FlacFiles:    flacFiles,
			SplitDone:    done,
			OutputTracks: outputTracks,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Path < results[j].Path })
	return results, nil
}

// CheckOutputStatus compares the number of FLAC files already present under
// outputDir/relPath against the track count parsed from cuePath, reporting
// whether the album looks fully split.
func CheckOutputStatus(outputDir, relPath, cuePath string) (done bool, outputTracks int) {
	outDir := filepath.Join(outputDir, relPath)
	info, err := os.Stat(outDir)
	if err != nil || !info.IsDir() {
		return false, 0
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		return false, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ".flac") {
			outputTracks++
		}
	}

	expected := 0
	if album, err := cue.Parse(cuePath); err == nil {
		expected = len(album.Tracks)
	}

	done = outputTracks > 0 && outputTracks >= expected && expected > 0
	return done, outputTracks
}

// Search filters pairs to those whose Path contains q (case-insensitive).
// An empty q reports no matches, mirroring the original endpoint.
func Search(pairs []Pair, q string) []Pair {
	q = strings.ToLower(q)
	filtered := []Pair{}
	if q == "" {
		return filtered
	}
	for _, p := range pairs {
		if strings.Contains(strings.ToLower(p.Path), q) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}
