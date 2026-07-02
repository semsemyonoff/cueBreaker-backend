package cue

import (
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

	t.Run("unreadable cue", func(t *testing.T) {
		dir := t.TempDir()
		if HasSourceFLAC(filepath.Join(dir, "does_not_exist.cue"), dir) {
			t.Error("HasSourceFLAC() = true, want false when the CUE cannot be read")
		}
	})
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
