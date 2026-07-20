package cue

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseText_MultiTrack(t *testing.T) {
	content := `PERFORMER "Album Artist"
TITLE "Album Title"
FILE "album.flac" WAVE
REM GENRE "Rock"
REM DATE 1999
  TRACK 01 AUDIO
    TITLE "First Track"
    PERFORMER "Track Artist"
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "Second Track"
    INDEX 01 03:45:20
`
	album := ParseText(content)

	if album.Performer != "Album Artist" || album.Title != "Album Title" || album.File != "album.flac" {
		t.Fatalf("album meta = %+v, want performer/title/file set", album)
	}
	if album.Genre != "Rock" || album.Date != "1999" {
		t.Fatalf("album genre/date = %q/%q, want Rock/1999", album.Genre, album.Date)
	}
	if len(album.Tracks) != 2 {
		t.Fatalf("len(Tracks) = %d, want 2", len(album.Tracks))
	}

	t1 := album.Tracks[0]
	if t1.Number != 1 || t1.Title != "First Track" || t1.Performer != "Track Artist" {
		t.Errorf("track 1 = %+v, want number=1 title=First Track performer=Track Artist", t1)
	}
	if t1.Index != "00:00:00" || t1.StartSeconds != 0 {
		t.Errorf("track 1 index/start = %q/%v, want 00:00:00/0", t1.Index, t1.StartSeconds)
	}

	t2 := album.Tracks[1]
	if t2.Number != 2 || t2.Title != "Second Track" {
		t.Errorf("track 2 = %+v, want number=2 title=Second Track", t2)
	}
	// track 2 has no PERFORMER line: defaults to album performer.
	if t2.Performer != "Album Artist" {
		t.Errorf("track 2 performer = %q, want default to album performer %q", t2.Performer, album.Performer)
	}
	wantStart := 3*60 + 45 + 20.0/75
	if t2.Index != "03:45:20" || t2.StartSeconds != wantStart {
		t.Errorf("track 2 index/start = %q/%v, want 03:45:20/%v", t2.Index, t2.StartSeconds, wantStart)
	}
}

func TestParseText_MissingFields(t *testing.T) {
	content := `TRACK 01 AUDIO
    INDEX 01 00:00:00
`
	album := ParseText(content)
	if album.Performer != "" || album.Title != "" || album.File != "" || album.Genre != "" || album.Date != "" {
		t.Errorf("album = %+v, want all-empty album metadata", album)
	}
	if len(album.Tracks) != 1 {
		t.Fatalf("len(Tracks) = %d, want 1", len(album.Tracks))
	}
	tr := album.Tracks[0]
	if tr.Title != "" || tr.Performer != "" {
		t.Errorf("track = %+v, want empty title/performer when absent and no album performer to default to", tr)
	}
}

func TestParseText_GenreDateQuoting(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantVal  string
		wantDate string
	}{
		{"quoted genre", `REM GENRE "Alternative Rock"` + "\nREM DATE 2003\n", "Alternative Rock", "2003"},
		{"unquoted genre", "REM GENRE Rock\nREM DATE 2003\n", "Rock", "2003"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			album := ParseText(tt.line)
			if album.Genre != tt.wantVal {
				t.Errorf("Genre = %q, want %q", album.Genre, tt.wantVal)
			}
			if album.Date != tt.wantDate {
				t.Errorf("Date = %q, want %q", album.Date, tt.wantDate)
			}
		})
	}
}

// TestParseText_DataTrackDropped pins that trackHeaderRe matches only
// `TRACK n AUDIO`: a data track (MODE1/2352) is silently dropped, and the
// surviving audio tracks keep the numbers the CUE gave them (so the parse
// is sparse — 1 and 3 — rather than renumbered 1 and 2). The dropped
// track's block is folded into the preceding audio track's block, so this
// also pins that the preceding track keeps its own TITLE/INDEX (firstMatch
// takes the first match in the block, not the data track's).
func TestParseText_DataTrackDropped(t *testing.T) {
	content := `PERFORMER "Album Artist"
TITLE "Album Title"
FILE "album.flac" WAVE
  TRACK 01 AUDIO
    TITLE "First Track"
    INDEX 01 00:00:00
  TRACK 02 MODE1/2352
    TITLE "Data Track"
    INDEX 01 03:00:00
  TRACK 03 AUDIO
    TITLE "Third Track"
    INDEX 01 05:00:00
`
	album := ParseText(content)

	if len(album.Tracks) != 2 {
		t.Fatalf("len(Tracks) = %d, want 2 (the MODE1/2352 data track is dropped): %+v", len(album.Tracks), album.Tracks)
	}
	if got := []int{album.Tracks[0].Number, album.Tracks[1].Number}; got[0] != 1 || got[1] != 3 {
		t.Errorf("track numbers = %v, want [1 3] — audio tracks keep their CUE numbering across a dropped data track", got)
	}

	t1 := album.Tracks[0]
	if t1.Title != "First Track" || t1.Index != "00:00:00" {
		t.Errorf("track 1 = %+v, want its own title/index, not the interleaved data track's", t1)
	}
	if album.Tracks[1].Title != "Third Track" {
		t.Errorf("track 3 title = %q, want Third Track", album.Tracks[1].Title)
	}
}

// TestParseText_LeadingDataTrack pins the same rule when the data track
// comes first: its block precedes every audio header, so it belongs to no
// track at all.
func TestParseText_LeadingDataTrack(t *testing.T) {
	content := `FILE "album.flac" WAVE
  TRACK 01 MODE1/2352
    TITLE "Data Track"
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "Second Track"
    INDEX 01 05:00:00
`
	album := ParseText(content)

	if len(album.Tracks) != 1 {
		t.Fatalf("len(Tracks) = %d, want 1 (leading data track dropped): %+v", len(album.Tracks), album.Tracks)
	}
	if album.Tracks[0].Number != 2 || album.Tracks[0].Title != "Second Track" {
		t.Errorf("track = %+v, want number=2 title=Second Track", album.Tracks[0])
	}
}

// TestParseText_IndexZeroOnly pins that trackIndexRe requires INDEX 01: a
// track carrying only the INDEX 00 pregap marker parses with an empty
// Index and a zero StartSeconds rather than adopting the pregap offset.
func TestParseText_IndexZeroOnly(t *testing.T) {
	content := `FILE "album.flac" WAVE
  TRACK 01 AUDIO
    TITLE "Pregap Only"
    INDEX 00 01:30:00
`
	album := ParseText(content)

	if len(album.Tracks) != 1 {
		t.Fatalf("len(Tracks) = %d, want 1", len(album.Tracks))
	}
	tr := album.Tracks[0]
	if tr.Index != "" || tr.StartSeconds != 0 {
		t.Errorf("track index/start = %q/%v, want \"\"/0 — INDEX 00 is not a track start", tr.Index, tr.StartSeconds)
	}
	if tr.Title != "Pregap Only" {
		t.Errorf("track title = %q, want Pregap Only (a missing INDEX 01 must not drop the track)", tr.Title)
	}
}

func TestIndexToSeconds(t *testing.T) {
	tests := []struct {
		index string
		want  float64
	}{
		{"00:00:00", 0},
		{"00:01:00", 1},
		{"01:00:00", 60},
		{"03:45:20", 3*60 + 45 + 20.0/75},
		{"bogus", 0},
		{"1:2", 0},
		// Frame counts are not range-checked: 75 frames is one whole second,
		// so an out-of-range frame simply rolls over rather than erroring.
		{"00:00:75", 1},
		{"00:00:150", 2},
		// Wrong part count in either direction yields 0.
		{"00:00:00:00", 0},
		{"00", 0},
		{"", 0},
		// Non-numeric parts yield 0, one bad part at a time.
		{"aa:00:00", 0},
		{"00:bb:00", 0},
		{"00:00:cc", 0},
	}
	for _, tt := range tests {
		if got := indexToSeconds(tt.index); got != tt.want {
			t.Errorf("indexToSeconds(%q) = %v, want %v", tt.index, got, tt.want)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestHasSourceFLAC(t *testing.T) {
	t.Run("single existing flac", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "album.flac"), "")
		cuePath := filepath.Join(dir, "album.cue")
		writeFile(t, cuePath, `FILE "album.flac" WAVE
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
		if !HasSourceFLAC(cuePath, dir) {
			t.Error("HasSourceFLAC() = false, want true for single existing FLAC ref")
		}
	})

	t.Run("single existing wav", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "album.wav"), "")
		cuePath := filepath.Join(dir, "album.cue")
		writeFile(t, cuePath, `FILE "album.wav" WAVE
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
		if !HasSourceFLAC(cuePath, dir) {
			t.Error("HasSourceFLAC() = false, want true for single existing WAV ref")
		}
	})

	t.Run("missing source file", func(t *testing.T) {
		dir := t.TempDir()
		cuePath := filepath.Join(dir, "album.cue")
		writeFile(t, cuePath, `FILE "album.flac" WAVE
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
		if HasSourceFLAC(cuePath, dir) {
			t.Error("HasSourceFLAC() = true, want false when the referenced file does not exist")
		}
	})

	t.Run("multi-file cue already split", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "01.flac"), "")
		writeFile(t, filepath.Join(dir, "02.flac"), "")
		cuePath := filepath.Join(dir, "album.cue")
		writeFile(t, cuePath, `FILE "01.flac" WAVE
TRACK 01 AUDIO
INDEX 01 00:00:00
FILE "02.flac" WAVE
TRACK 02 AUDIO
INDEX 01 00:00:00
`)
		if HasSourceFLAC(cuePath, dir) {
			t.Error("HasSourceFLAC() = true, want false for a multi-file (already split) CUE")
		}
	})

	t.Run("non-audio file ref", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "album.mp3"), "")
		cuePath := filepath.Join(dir, "album.cue")
		writeFile(t, cuePath, `FILE "album.mp3" MP3
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
		if HasSourceFLAC(cuePath, dir) {
			t.Error("HasSourceFLAC() = true, want false for a non-flac/wav FILE reference")
		}
	})

	t.Run("cue with no FILE reference", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "album.flac"), "")
		cuePath := filepath.Join(dir, "album.cue")
		writeFile(t, cuePath, `PERFORMER "Album Artist"
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
		// fileReferences returns 0 refs, and len(refs) != 1 rejects: an album
		// is only a split candidate when its CUE names exactly one source.
		if HasSourceFLAC(cuePath, dir) {
			t.Error("HasSourceFLAC() = true, want false for a CUE with no FILE reference, even with a FLAC beside it")
		}
	})

	t.Run("unreadable cue", func(t *testing.T) {
		dir := t.TempDir()
		if HasSourceFLAC(filepath.Join(dir, "does_not_exist.cue"), dir) {
			t.Error("HasSourceFLAC() = true, want false when the CUE cannot be read")
		}
	})

	t.Run("FILE reference escapes dir via ..", func(t *testing.T) {
		parent := t.TempDir()
		writeFile(t, filepath.Join(parent, "outside.flac"), "")
		dir := filepath.Join(parent, "album")
		cuePath := filepath.Join(dir, "album.cue")
		writeFile(t, cuePath, `FILE "../outside.flac" WAVE
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
		if HasSourceFLAC(cuePath, dir) {
			t.Error("HasSourceFLAC() = true, want false for a FILE reference that escapes dir via ..")
		}
	})

	t.Run("symlinked source escapes dir", func(t *testing.T) {
		outside := t.TempDir()
		writeFile(t, filepath.Join(outside, "real.flac"), "")
		dir := t.TempDir()
		if err := os.Symlink(filepath.Join(outside, "real.flac"), filepath.Join(dir, "album.flac")); err != nil {
			t.Skipf("symlink not supported: %v", err)
		}
		cuePath := filepath.Join(dir, "album.cue")
		writeFile(t, cuePath, `FILE "album.flac" WAVE
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
		if HasSourceFLAC(cuePath, dir) {
			t.Error("HasSourceFLAC() = true, want false for a symlink that escapes dir")
		}
	})
}

func TestCheckSourceFLAC(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) (cuePath, dir string)
		wantErr error
	}{
		{
			name: "unreadable cue",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				return filepath.Join(dir, "does_not_exist.cue"), dir
			},
			wantErr: ErrCUEUnreadable,
		},
		{
			name: "no FILE reference",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "album.flac"), "")
				cuePath := filepath.Join(dir, "album.cue")
				writeFile(t, cuePath, `PERFORMER "Album Artist"
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
				return cuePath, dir
			},
			wantErr: ErrNoFileReference,
		},
		{
			name: "multiple FILE references",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "01.flac"), "")
				writeFile(t, filepath.Join(dir, "02.flac"), "")
				cuePath := filepath.Join(dir, "album.cue")
				writeFile(t, cuePath, `FILE "01.flac" WAVE
TRACK 01 AUDIO
INDEX 01 00:00:00
FILE "02.flac" WAVE
TRACK 02 AUDIO
INDEX 01 00:00:00
`)
				return cuePath, dir
			},
			wantErr: ErrMultiFileReference,
		},
		{
			name: "non-FLAC/WAV reference",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "album.mp3"), "")
				cuePath := filepath.Join(dir, "album.cue")
				writeFile(t, cuePath, `FILE "album.mp3" MP3
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
				return cuePath, dir
			},
			wantErr: ErrNotFLACOrWAV,
		},
		{
			name: "missing source file",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				cuePath := filepath.Join(dir, "album.cue")
				writeFile(t, cuePath, `FILE "album.flac" WAVE
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
				return cuePath, dir
			},
			wantErr: ErrSourceMissing,
		},
		{
			name: "source escapes dir via ..",
			setup: func(t *testing.T) (string, string) {
				parent := t.TempDir()
				writeFile(t, filepath.Join(parent, "outside.flac"), "")
				dir := filepath.Join(parent, "album")
				cuePath := filepath.Join(dir, "album.cue")
				writeFile(t, cuePath, `FILE "../outside.flac" WAVE
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
				return cuePath, dir
			},
			wantErr: ErrSourceMissing,
		},
		{
			name: "valid single-file cue",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "album.flac"), "")
				cuePath := filepath.Join(dir, "album.cue")
				writeFile(t, cuePath, `FILE "album.flac" WAVE
TRACK 01 AUDIO
INDEX 01 00:00:00
`)
				return cuePath, dir
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cuePath, dir := tt.setup(t)
			err := CheckSourceFLAC(cuePath, dir)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("CheckSourceFLAC() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("CheckSourceFLAC() = %v, want error matching %v", err, tt.wantErr)
			}
		})
	}
}

// TestHasSourceFLAC_AgreesWithCheckSourceFLAC pins that HasSourceFLAC's
// bool result agrees with CheckSourceFLAC's error across every fixture in
// TestHasSourceFLAC, since HasSourceFLAC is now a one-line wrapper over it.
func TestHasSourceFLAC_AgreesWithCheckSourceFLAC(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "album.flac"), "")
	cuePath := filepath.Join(dir, "album.cue")
	writeFile(t, cuePath, `FILE "album.flac" WAVE
TRACK 01 AUDIO
INDEX 01 00:00:00
`)

	if got, want := HasSourceFLAC(cuePath, dir), CheckSourceFLAC(cuePath, dir) == nil; got != want {
		t.Errorf("HasSourceFLAC() = %v, want %v (agreement with CheckSourceFLAC)", got, want)
	}
	if !HasSourceFLAC(cuePath, dir) {
		t.Error("HasSourceFLAC() = false, want true for a valid single-file cue")
	}

	missingCue := filepath.Join(dir, "does_not_exist.cue")
	if got, want := HasSourceFLAC(missingCue, dir), CheckSourceFLAC(missingCue, dir) == nil; got != want {
		t.Errorf("HasSourceFLAC() = %v, want %v (agreement with CheckSourceFLAC)", got, want)
	}
	if HasSourceFLAC(missingCue, dir) {
		t.Error("HasSourceFLAC() = true, want false when the CUE cannot be read")
	}
}

func TestSourceFLAC(t *testing.T) {
	t.Run("resolves FILE reference", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "album.flac"), "")
		album := Album{File: "album.flac"}

		got, ok := SourceFLAC(album, dir)
		if !ok {
			t.Fatal("SourceFLAC() ok = false, want true")
		}
		if got != filepath.Join(dir, "album.flac") {
			t.Errorf("SourceFLAC() = %q, want %q", got, filepath.Join(dir, "album.flac"))
		}
	})

	t.Run("falls back to first flac in dir", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "disc.flac"), "")
		album := Album{File: "missing.flac"}

		got, ok := SourceFLAC(album, dir)
		if !ok {
			t.Fatal("SourceFLAC() ok = false, want true (fallback to first FLAC)")
		}
		if got != filepath.Join(dir, "disc.flac") {
			t.Errorf("SourceFLAC() = %q, want %q", got, filepath.Join(dir, "disc.flac"))
		}
	})

	t.Run("falls back to wav when no flac", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "disc.wav"), "")
		album := Album{}

		got, ok := SourceFLAC(album, dir)
		if !ok {
			t.Fatal("SourceFLAC() ok = false, want true (fallback to WAV)")
		}
		if got != filepath.Join(dir, "disc.wav") {
			t.Errorf("SourceFLAC() = %q, want %q", got, filepath.Join(dir, "disc.wav"))
		}
	})

	t.Run("none found", func(t *testing.T) {
		dir := t.TempDir()
		album := Album{}

		if _, ok := SourceFLAC(album, dir); ok {
			t.Error("SourceFLAC() ok = true, want false when no source audio file exists")
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()
		writeFile(t, filepath.Join(outside, "secret.flac"), "")

		rel, err := filepath.Rel(dir, filepath.Join(outside, "secret.flac"))
		if err != nil {
			t.Fatalf("filepath.Rel: %v", err)
		}
		album := Album{File: rel}

		if _, ok := SourceFLAC(album, dir); ok {
			t.Error("SourceFLAC() ok = true, want false for a FILE reference that escapes dir via ..")
		}
	})

	t.Run("rejects symlink escape", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()
		writeFile(t, filepath.Join(outside, "secret.flac"), "")

		link := filepath.Join(dir, "album.flac")
		if err := os.Symlink(filepath.Join(outside, "secret.flac"), link); err != nil {
			t.Skipf("symlink not supported: %v", err)
		}
		album := Album{File: "album.flac"}

		if _, ok := SourceFLAC(album, dir); ok {
			t.Error("SourceFLAC() ok = true, want false for a symlink that escapes dir")
		}
	})
}

func TestParse_ReadsRealFixture(t *testing.T) {
	album, err := Parse(testdataPath(t, "utf8.cue"))
	if err != nil {
		t.Fatalf("Parse() returned error: %v", err)
	}
	if album.Performer != "Test Artist" || album.Title != "Test Album" {
		t.Errorf("album = %+v, want performer=Test Artist title=Test Album", album)
	}
	if len(album.Tracks) != 2 {
		t.Fatalf("len(Tracks) = %d, want 2", len(album.Tracks))
	}
}

func TestParse_MissingFile(t *testing.T) {
	if _, err := Parse(testdataPath(t, "does_not_exist.cue")); err == nil {
		t.Error("Parse() with missing file: expected error, got nil")
	}
}
