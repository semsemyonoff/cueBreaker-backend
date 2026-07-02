package scan

import (
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
