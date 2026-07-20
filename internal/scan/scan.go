package scan

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"git.horn/cueBreaker/backend/internal/cue"
	"git.horn/cueBreaker/backend/internal/joblog"
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

// Result is the outcome of a walk: the pairs found, a log of every rejected
// directory and why, and a summary of the walk.
type Result struct {
	Pairs   []Pair         `json:"items"`
	Log     []joblog.Entry `json:"log"`
	Summary Summary        `json:"summary"`
}

// Summary counts the outcome of a walk.
type Summary struct {
	DirsWalked int   `json:"dirs_walked"`
	Albums     int   `json:"albums"`
	Unsplit    int   `json:"unsplit"`
	Skipped    int   `json:"skipped"`
	ElapsedMs  int64 `json:"elapsed_ms"`
}

// FindPairs walks inputDir and returns every directory containing at least
// one CUE sheet that references an existing single source FLAC/WAV (i.e. an
// unsplit album), sorted by relative path. A directory whose CUE sheets are
// all multi-file (already split) or reference a missing source is skipped,
// and — provided the directory held at least one .cue — logged with the
// reason from cue.CheckSourceFLAC. Directories with no .cue at all produce
// no log entry: they are the overwhelming majority of a walk and carry no
// signal.
func FindPairs(inputDir, outputDir string) (Result, error) {
	start := time.Now()
	log := joblog.New(0)
	log.Add(joblog.LevelInfo, "scanning %s", inputDir)

	// Initialize non-nil so an empty library marshals to JSON [] rather than
	// null; the SPA treats the response as an array (items.length) and would
	// otherwise crash on the first-run empty scan.
	results := []Pair{}
	dirsWalked := 0
	skipped := 0

	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A single unreadable entry (odd perms, transient mount, lost+found)
			// shouldn't blank the whole library; skip it and keep walking. Only
			// an unreadable input root is fatal.
			if path == inputDir {
				return err
			}
			log.Add(joblog.LevelWarn, "skip %s — unreadable: %s", relOrSelf(inputDir, path), reason(err))
			skipped++
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		dirsWalked++

		entries, err := os.ReadDir(path)
		if err != nil {
			// An unreadable input root is fatal here too: WalkDir turns a
			// SkipDir returned from the root callback into a nil error, so
			// without this the caller would see a successful empty scan and
			// the SPA would render a broken bind mount as an empty library.
			if path == inputDir {
				return err
			}
			log.Add(joblog.LevelWarn, "skip %s — unreadable: %s", relOrSelf(inputDir, path), reason(err))
			skipped++
			// WalkDir would otherwise read this directory itself, fail the same
			// way, and re-enter this callback with the error — logging the line
			// and bumping skipped a second time. SkipDir says "already handled".
			return fs.SkipDir
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
		sort.Strings(cueFiles)

		relPath, err := filepath.Rel(inputDir, path)
		if err != nil {
			return err
		}

		var validCues []string
		var firstRejectErr error
		for _, cf := range cueFiles {
			checkErr := cue.CheckSourceFLAC(filepath.Join(path, cf), path)
			if checkErr != nil {
				if firstRejectErr == nil {
					firstRejectErr = checkErr
				}
				continue
			}
			validCues = append(validCues, cf)
		}
		if len(validCues) == 0 {
			log.Add(rejectLevel(firstRejectErr), "skip %s — %s", relPath, firstRejectErr)
			skipped++
			return nil
		}
		sort.Strings(flacFiles)

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
		return Result{}, err
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Path < results[j].Path })

	unsplit := 0
	for _, p := range results {
		if !p.SplitDone {
			unsplit++
		}
	}
	elapsed := time.Since(start)

	log.Add(joblog.LevelInfo, "scanned %d dirs · %d albums · %d unsplit · %d skipped · %dms",
		dirsWalked, len(results), unsplit, skipped, elapsed.Milliseconds())
	entries, _ := log.Since(0)

	return Result{
		Pairs: results,
		Log:   entries,
		Summary: Summary{
			DirsWalked: dirsWalked,
			Albums:     len(results),
			Unsplit:    unsplit,
			Skipped:    skipped,
			ElapsedMs:  elapsed.Milliseconds(),
		},
	}, nil
}

// rejectLevel classifies a cue.CheckSourceFLAC error into the log level a
// scan rejection line should carry: info for outcomes that are expected
// steady-state (already split, non-audio source), warn for everything that
// suggests something is actually wrong with the directory or CUE.
func rejectLevel(err error) joblog.Level {
	if errors.Is(err, cue.ErrMultiFileReference) || errors.Is(err, cue.ErrNotFLACOrWAV) {
		return joblog.LevelInfo
	}
	return joblog.LevelWarn
}

// reason reduces a filesystem error to its underlying cause ("permission
// denied") without the absolute path *fs.PathError carries. The scan log
// already names the directory relative to the input root; repeating it as a
// host-absolute path is noise the SPA has no use for.
func reason(err error) string {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err.Error()
	}
	return err.Error()
}

// relOrSelf returns path relative to base, falling back to path itself if
// the relative form cannot be computed.
func relOrSelf(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
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
