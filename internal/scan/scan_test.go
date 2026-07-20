package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.horn/cueBreaker/backend/internal/joblog"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

const twoTrackCue = `PERFORMER "Album Artist"
TITLE "Album Title"
FILE "album.flac" WAVE
  TRACK 01 AUDIO
    TITLE "First"
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "Second"
    INDEX 01 03:00:00
`

const multiFileCue = `PERFORMER "Album Artist"
TITLE "Album Title"
FILE "01 - First.flac" WAVE
  TRACK 01 AUDIO
    TITLE "First"
    INDEX 01 00:00:00
FILE "02 - Second.flac" WAVE
  TRACK 02 AUDIO
    TITLE "Second"
    INDEX 01 00:00:00
`

const wavSourceCue = `PERFORMER "Album Artist"
TITLE "Album Title"
FILE "album.wav" WAVE
  TRACK 01 AUDIO
    TITLE "First"
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "Second"
    INDEX 01 03:00:00
`

func TestFindPairs_UnsplitPair(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	writeFile(t, filepath.Join(input, "Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(input, "Album", "album.flac"), "fake-flac")

	result, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	pairs := result.Pairs
	if len(pairs) != 1 {
		t.Fatalf("len(pairs) = %d, want 1: %+v", len(pairs), pairs)
	}

	p := pairs[0]
	if p.Path != "Album" {
		t.Errorf("Path = %q, want Album", p.Path)
	}
	if len(p.CueFiles) != 1 || p.CueFiles[0] != "album.cue" {
		t.Errorf("CueFiles = %v, want [album.cue]", p.CueFiles)
	}
	if len(p.FlacFiles) != 1 || p.FlacFiles[0] != "album.flac" {
		t.Errorf("FlacFiles = %v, want [album.flac]", p.FlacFiles)
	}
	if p.SplitDone {
		t.Errorf("SplitDone = true, want false (no output dir)")
	}
	if p.OutputTracks != 0 {
		t.Errorf("OutputTracks = %d, want 0", p.OutputTracks)
	}
}

func TestFindPairs_AlreadySplit(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	writeFile(t, filepath.Join(input, "Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(input, "Album", "album.flac"), "fake-flac")

	writeFile(t, filepath.Join(output, "Album", "01 - First.flac"), "x")
	writeFile(t, filepath.Join(output, "Album", "02 - Second.flac"), "x")

	result, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	pairs := result.Pairs
	if len(pairs) != 1 {
		t.Fatalf("len(pairs) = %d, want 1", len(pairs))
	}
	if !pairs[0].SplitDone {
		t.Errorf("SplitDone = false, want true")
	}
	if pairs[0].OutputTracks != 2 {
		t.Errorf("OutputTracks = %d, want 2", pairs[0].OutputTracks)
	}
}

func TestFindPairs_MultiFileCueSkipped(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	writeFile(t, filepath.Join(input, "Album", "album.cue"), multiFileCue)
	writeFile(t, filepath.Join(input, "Album", "01 - First.flac"), "x")
	writeFile(t, filepath.Join(input, "Album", "02 - Second.flac"), "x")

	result, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	if len(result.Pairs) != 0 {
		t.Fatalf("len(pairs) = %d, want 0 (multi-file cue is already split, not a candidate)", len(result.Pairs))
	}
}

func TestFindPairs_NestedDirs(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	writeFile(t, filepath.Join(input, "Artist", "Album One", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(input, "Artist", "Album One", "album.flac"), "x")
	writeFile(t, filepath.Join(input, "Artist", "Album Two", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(input, "Artist", "Album Two", "album.flac"), "x")

	result, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	pairs := result.Pairs
	if len(pairs) != 2 {
		t.Fatalf("len(pairs) = %d, want 2: %+v", len(pairs), pairs)
	}
	if pairs[0].Path != filepath.Join("Artist", "Album One") {
		t.Errorf("pairs[0].Path = %q, want Artist/Album One", pairs[0].Path)
	}
	if pairs[1].Path != filepath.Join("Artist", "Album Two") {
		t.Errorf("pairs[1].Path = %q, want Artist/Album Two", pairs[1].Path)
	}
}

func TestFindPairs_NoCueSkipped(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	writeFile(t, filepath.Join(input, "NotAnAlbum", "readme.txt"), "hi")

	result, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	if len(result.Pairs) != 0 {
		t.Fatalf("len(pairs) = %d, want 0", len(result.Pairs))
	}
}

// A directory with no .cue at all is noise for the overwhelming majority of
// a walk and must never produce a log entry — only Task 6's zero-`.cue`
// early return path exercises this, the rejection path never gets a look.
func TestFindPairs_NoCueProducesNoLogEntry(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	writeFile(t, filepath.Join(input, "NotAnAlbum", "readme.txt"), "hi")

	result, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	for _, e := range result.Log {
		if strings.Contains(e.Text, "NotAnAlbum") {
			t.Errorf("log entry %+v mentions NotAnAlbum, want no entry for a directory with no .cue", e)
		}
	}
	if result.Summary.Skipped != 0 {
		t.Errorf("Summary.Skipped = %d, want 0", result.Summary.Skipped)
	}
}

// A nil slice marshals to JSON null, which crashes the SPA (items.length);
// FindPairs must return a non-nil empty Pairs slice for an empty library,
// with Summary counters reflecting the empty walk.
func TestFindPairs_EmptyIsNonNil(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	result, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	if result.Pairs == nil {
		t.Fatal("FindPairs returned nil Pairs; want non-nil empty slice")
	}
	if result.Summary.Albums != 0 || result.Summary.Unsplit != 0 || result.Summary.Skipped != 0 {
		t.Errorf("Summary = %+v, want all-zero counters for an empty library", result.Summary)
	}
	if result.Summary.DirsWalked != 1 {
		t.Errorf("Summary.DirsWalked = %d, want 1 (the input root itself)", result.Summary.DirsWalked)
	}
}

// TestFindPairs_Summary drives a small library with one split album, one
// unsplit album and one rejected directory, and asserts every Summary
// counter against it.
func TestFindPairs_Summary(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	writeFile(t, filepath.Join(input, "Unsplit Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(input, "Unsplit Album", "album.flac"), "fake-flac")

	writeFile(t, filepath.Join(input, "Split Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(input, "Split Album", "album.flac"), "fake-flac")
	writeFile(t, filepath.Join(output, "Split Album", "01 - First.flac"), "x")
	writeFile(t, filepath.Join(output, "Split Album", "02 - Second.flac"), "x")

	writeFile(t, filepath.Join(input, "Rejected", "album.cue"), multiFileCue)

	result, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	want := Summary{DirsWalked: 4, Albums: 2, Unsplit: 1, Skipped: 1}
	if result.Summary.DirsWalked != want.DirsWalked || result.Summary.Albums != want.Albums ||
		result.Summary.Unsplit != want.Unsplit || result.Summary.Skipped != want.Skipped {
		t.Fatalf("Summary = %+v, want %+v (ElapsedMs elided)", result.Summary, want)
	}
	if result.Summary.ElapsedMs < 0 {
		t.Errorf("Summary.ElapsedMs = %d, want >= 0", result.Summary.ElapsedMs)
	}
}

// TestFindPairs_RejectionReasons is table-driven over one fixture per
// cue.CheckSourceFLAC failure reason, asserting the scan log carries the
// expected level and reason text, and that the directory produces no Pair.
func TestFindPairs_RejectionReasons(t *testing.T) {
	const noFileRefCue = `not a valid cue sheet, no FILE reference at all`

	tests := []struct {
		name      string
		cueBody   string
		wantLevel joblog.Level
		wantText  string
	}{
		{
			name:      "no FILE reference",
			cueBody:   noFileRefCue,
			wantLevel: joblog.LevelWarn,
			wantText:  "no FILE reference in album.cue",
		},
		{
			name:      "multi-file cue",
			cueBody:   multiFileCue,
			wantLevel: joblog.LevelInfo,
			wantText:  "multi-file cue (already split): album.cue",
		},
		{
			name: "source is not FLAC/WAV",
			cueBody: `FILE "album.mp3" WAVE
  TRACK 01 AUDIO
    INDEX 01 00:00:00
`,
			wantLevel: joblog.LevelInfo,
			wantText:  "source is not FLAC/WAV: album.mp3",
		},
		{
			name: "source file missing",
			cueBody: `FILE "album.flac" WAVE
  TRACK 01 AUDIO
    INDEX 01 00:00:00
`,
			wantLevel: joblog.LevelWarn,
			wantText:  "source file missing: album.flac",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := t.TempDir()
			output := t.TempDir()

			writeFile(t, filepath.Join(input, "Album", "album.cue"), tt.cueBody)

			result, err := FindPairs(input, output)
			if err != nil {
				t.Fatalf("FindPairs: %v", err)
			}
			if len(result.Pairs) != 0 {
				t.Fatalf("len(Pairs) = %d, want 0: %+v", len(result.Pairs), result.Pairs)
			}

			var found *joblog.Entry
			for i, e := range result.Log {
				if strings.Contains(e.Text, "Album") {
					found = &result.Log[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("no log entry mentioning Album, log = %+v", result.Log)
			}
			if found.Level != tt.wantLevel {
				t.Errorf("Level = %q, want %q", found.Level, tt.wantLevel)
			}
			wantSuffix := "skip Album — " + tt.wantText
			if found.Text != wantSuffix {
				t.Errorf("Text = %q, want %q", found.Text, wantSuffix)
			}
		})
	}
}

// A CUE that cannot be read at all (unreadable, not just unparseable — CUE
// parsing never errors) needs a file that fails os.ReadFile, since ReadCUE's
// only failure path is the underlying read.
func TestFindPairs_RejectionReason_CueUnreadable(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	cueDir := filepath.Join(input, "Album")
	if err := os.MkdirAll(cueDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// A broken symlink has a ".cue" name (so it is collected as a candidate,
	// unlike a directory of the same name, which ReadDir's IsDir() filter
	// would exclude before CheckSourceFLAC ever runs) but os.ReadFile fails
	// to follow it.
	if err := os.Symlink(filepath.Join(cueDir, "does-not-exist"), filepath.Join(cueDir, "album.cue")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	result, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	if len(result.Pairs) != 0 {
		t.Fatalf("len(Pairs) = %d, want 0", len(result.Pairs))
	}

	var found *joblog.Entry
	for i, e := range result.Log {
		if strings.Contains(e.Text, "Album") {
			found = &result.Log[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no log entry mentioning Album, log = %+v", result.Log)
	}
	if found.Level != joblog.LevelWarn {
		t.Errorf("Level = %q, want warn", found.Level)
	}
	if !strings.HasPrefix(found.Text, "skip Album — cue unreadable: album.cue:") {
		t.Errorf("Text = %q, want prefix %q", found.Text, "skip Album — cue unreadable: album.cue:")
	}
}

// FindPairs reaches CheckOutputStatus only through a full walk, which can
// only ever exercise the states a real library produces. These drive it
// directly, including the ones that decide "done" is false.
func TestCheckOutputStatus(t *testing.T) {
	// seed writes a CUE parsing to expected tracks and count FLACs into the
	// output dir, returning the output root and the cue path.
	seed := func(t *testing.T, cueContent string, outputTracks int) (output, cuePath string) {
		t.Helper()
		input := t.TempDir()
		output = t.TempDir()
		cuePath = filepath.Join(input, "Album", "album.cue")
		writeFile(t, cuePath, cueContent)
		for i := 1; i <= outputTracks; i++ {
			writeFile(t, filepath.Join(output, "Album", fmt.Sprintf("%02d - Track.flac", i)), "audio")
		}
		return output, cuePath
	}

	t.Run("fully split", func(t *testing.T) {
		output, cuePath := seed(t, twoTrackCue, 2)
		done, tracks := CheckOutputStatus(output, "Album", cuePath)
		if !done || tracks != 2 {
			t.Fatalf("CheckOutputStatus() = (%v, %d), want (true, 2)", done, tracks)
		}
	})

	t.Run("more output than expected still counts as done", func(t *testing.T) {
		// outputTracks >= expected, not ==: a stray extra FLAC (a bonus track,
		// a manual addition) must not re-open a finished album.
		output, cuePath := seed(t, twoTrackCue, 3)
		done, tracks := CheckOutputStatus(output, "Album", cuePath)
		if !done || tracks != 3 {
			t.Fatalf("CheckOutputStatus() = (%v, %d), want (true, 3)", done, tracks)
		}
	})

	t.Run("partial output is not done", func(t *testing.T) {
		output, cuePath := seed(t, twoTrackCue, 1)
		done, tracks := CheckOutputStatus(output, "Album", cuePath)
		if done || tracks != 1 {
			t.Fatalf("CheckOutputStatus() = (%v, %d), want (false, 1)", done, tracks)
		}
	})

	t.Run("no output tracks is not done", func(t *testing.T) {
		output, cuePath := seed(t, twoTrackCue, 0)
		// The album's output directory exists but holds nothing.
		if err := os.MkdirAll(filepath.Join(output, "Album"), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		done, tracks := CheckOutputStatus(output, "Album", cuePath)
		if done || tracks != 0 {
			t.Fatalf("CheckOutputStatus() = (%v, %d), want (false, 0)", done, tracks)
		}
	})

	t.Run("unparseable cue expects zero tracks and is never done", func(t *testing.T) {
		// expected == 0 must not make any output count as complete —
		// otherwise an unreadable CUE would mark every album split.
		output, cuePath := seed(t, "not a cue sheet at all", 2)
		done, tracks := CheckOutputStatus(output, "Album", cuePath)
		if done || tracks != 2 {
			t.Fatalf("CheckOutputStatus() = (%v, %d), want (false, 2)", done, tracks)
		}
	})

	t.Run("missing output dir", func(t *testing.T) {
		output, cuePath := seed(t, twoTrackCue, 0)
		done, tracks := CheckOutputStatus(output, "Album", cuePath)
		if done || tracks != 0 {
			t.Fatalf("CheckOutputStatus() = (%v, %d), want (false, 0)", done, tracks)
		}
	})

	t.Run("output path is a file, not a dir", func(t *testing.T) {
		output, cuePath := seed(t, twoTrackCue, 0)
		writeFile(t, filepath.Join(output, "Album"), "not a directory")
		done, tracks := CheckOutputStatus(output, "Album", cuePath)
		if done || tracks != 0 {
			t.Fatalf("CheckOutputStatus() = (%v, %d), want (false, 0)", done, tracks)
		}
	})

	t.Run("subdirectories and non-flac files are not counted", func(t *testing.T) {
		output, cuePath := seed(t, twoTrackCue, 2)
		writeFile(t, filepath.Join(output, "Album", "cover.jpg"), "img")
		writeFile(t, filepath.Join(output, "Album", "nested", "03 - Track.flac"), "audio")
		done, tracks := CheckOutputStatus(output, "Album", cuePath)
		if !done || tracks != 2 {
			t.Fatalf("CheckOutputStatus() = (%v, %d), want (true, 2)", done, tracks)
		}
	})
}

// TestFindPairs_WavSourceHasEmptyFlacFiles pins today's asymmetry rather
// than blessing it: cue.HasSourceFLAC accepts a .wav FILE reference (and
// cue.TotalSeconds can read one), so a WAV-sourced album *does* list as an
// unsplit pair — but FindPairs only ever collects .flac names, so the pair
// comes back with an empty FlacFiles. Supporting WAV end-to-end is a product
// decision; this test exists so the current behaviour cannot drift silently.
func TestFindPairs_WavSourceHasEmptyFlacFiles(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	albumDir := filepath.Join(input, "Artist", "Wav Album")
	writeFile(t, filepath.Join(albumDir, "album.cue"), wavSourceCue)
	writeFile(t, filepath.Join(albumDir, "album.wav"), "fake audio")

	result, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	pairs := result.Pairs
	if len(pairs) != 1 {
		t.Fatalf("len(pairs) = %d, want 1 — a WAV-sourced CUE is still a valid unsplit pair: %+v", len(pairs), pairs)
	}
	if got := pairs[0].CueFiles; len(got) != 1 || got[0] != "album.cue" {
		t.Errorf("CueFiles = %v, want [album.cue]", got)
	}
	if len(pairs[0].FlacFiles) != 0 {
		t.Errorf("FlacFiles = %v, want empty: FindPairs collects only .flac names, so album.wav is not listed", pairs[0].FlacFiles)
	}
}

func TestSearch(t *testing.T) {
	pairs := []Pair{
		{Path: "Artist/Album One"},
		{Path: "Artist/Album Two"},
		{Path: "Other/Thing"},
	}

	t.Run("match", func(t *testing.T) {
		got := Search(pairs, "album")
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2: %+v", len(got), got)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		got := Search(pairs, "ARTIST")
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := Search(pairs, "nonexistent")
		if len(got) != 0 {
			t.Fatalf("len(got) = %d, want 0", len(got))
		}
	})

	t.Run("empty query", func(t *testing.T) {
		got := Search(pairs, "")
		if len(got) != 0 {
			t.Fatalf("len(got) = %d, want 0 (empty query yields no results)", len(got))
		}
	})
}

// TestFindPairs_UnreadableDirectory covers the walk's error branch: an
// unreadable directory is logged once at warn level with a path relative to
// the input root, counted once in Summary.Skipped, and does not abort the
// walk — a sibling album is still found.
func TestFindPairs_UnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 0o000 does not deny reads")
	}

	input := t.TempDir()
	output := t.TempDir()

	// A well-formed album that must survive the unreadable sibling.
	writeFile(t, filepath.Join(input, "Good", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(input, "Good", "album.flac"), "")

	locked := filepath.Join(input, "Locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	result, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}

	if len(result.Pairs) != 1 || result.Pairs[0].Path != "Good" {
		t.Fatalf("Pairs = %+v, want just the readable Good album", result.Pairs)
	}
	if result.Summary.Skipped != 1 {
		t.Errorf("Summary.Skipped = %d, want 1 (the unreadable directory counted once)", result.Summary.Skipped)
	}

	var matched []joblog.Entry
	for _, e := range result.Log {
		if strings.Contains(e.Text, "Locked") {
			matched = append(matched, e)
		}
	}
	if len(matched) != 1 {
		t.Fatalf("log lines mentioning Locked = %+v, want exactly 1", matched)
	}
	if matched[0].Level != joblog.LevelWarn {
		t.Errorf("level = %v, want %v", matched[0].Level, joblog.LevelWarn)
	}
	if !strings.Contains(matched[0].Text, "skip Locked — unreadable") {
		t.Errorf("text = %q, want a relative-path skip line", matched[0].Text)
	}
	if strings.Contains(matched[0].Text, input) {
		t.Errorf("text = %q, want the path relative to the input root, not absolute", matched[0].Text)
	}
}
