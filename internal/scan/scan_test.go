package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
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

	pairs, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
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

	pairs, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
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

	pairs, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	if len(pairs) != 0 {
		t.Fatalf("len(pairs) = %d, want 0 (multi-file cue is already split, not a candidate)", len(pairs))
	}
}

func TestFindPairs_NestedDirs(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	writeFile(t, filepath.Join(input, "Artist", "Album One", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(input, "Artist", "Album One", "album.flac"), "x")
	writeFile(t, filepath.Join(input, "Artist", "Album Two", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(input, "Artist", "Album Two", "album.flac"), "x")

	pairs, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
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

	pairs, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	if len(pairs) != 0 {
		t.Fatalf("len(pairs) = %d, want 0", len(pairs))
	}
}

// A nil slice marshals to JSON null, which crashes the SPA (items.length);
// FindPairs must return a non-nil empty slice for an empty library.
func TestFindPairs_EmptyIsNonNil(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	pairs, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
	if pairs == nil {
		t.Fatal("FindPairs returned nil slice; want non-nil empty slice")
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

	pairs, err := FindPairs(input, output)
	if err != nil {
		t.Fatalf("FindPairs: %v", err)
	}
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
