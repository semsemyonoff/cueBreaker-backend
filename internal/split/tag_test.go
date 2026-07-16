package split

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"git.horn/cueBreaker/backend/internal/cue"
)

func TestBuildTags(t *testing.T) {
	tests := []struct {
		name string
		in   trackTagFields
		want map[string]string
	}{
		{
			name: "all fields present",
			in: trackTagFields{
				Title: "Track One", Artist: "Artist", TrackNumber: "01",
				Album: "Album", AlbumArtist: "Album Artist",
				Genre: "Rock", Date: "2020", TrackTotal: 2,
			},
			want: map[string]string{
				"TITLE": "Track One", "ARTIST": "Artist", "TRACKNUMBER": "01",
				"ALBUM": "Album", "ALBUMARTIST": "Album Artist",
				"GENRE": "Rock", "DATE": "2020", "TRACKTOTAL": "2",
			},
		},
		{
			name: "missing cueprint fields are omitted",
			in: trackTagFields{
				Genre: "Jazz", Date: "1999", TrackTotal: 5,
			},
			want: map[string]string{
				"GENRE": "Jazz", "DATE": "1999", "TRACKTOTAL": "5",
			},
		},
		{
			name: "missing genre and date are omitted",
			in: trackTagFields{
				Title: "Solo", TrackTotal: 1,
			},
			want: map[string]string{
				"TITLE": "Solo", "TRACKTOTAL": "1",
			},
		},
		{
			name: "TRACKTOTAL always present even at zero",
			in:   trackTagFields{},
			want: map[string]string{"TRACKTOTAL": "0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTags(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPartitionPregap(t *testing.T) {
	tests := []struct {
		name       string
		in         []string
		wantReal   []string
		wantPregap []string
	}{
		{
			name:       "pregap first",
			in:         []string{"00 - pregap.flac", "01 - First.flac", "02 - Second.flac"},
			wantReal:   []string{"01 - First.flac", "02 - Second.flac"},
			wantPregap: []string{"00 - pregap.flac"},
		},
		{
			name:     "no pregap",
			in:       []string{"01 - First.flac", "02 - Second.flac"},
			wantReal: []string{"01 - First.flac", "02 - Second.flac"},
		},
		{
			name:       "all pregap",
			in:         []string{"00 - pregap.flac"},
			wantPregap: []string{"00 - pregap.flac"},
		},
		{
			name: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReal, gotPregap := partitionPregap(tt.in)
			if !reflect.DeepEqual(gotReal, tt.wantReal) {
				t.Fatalf("real = %v, want %v", gotReal, tt.wantReal)
			}
			if !reflect.DeepEqual(gotPregap, tt.wantPregap) {
				t.Fatalf("pregap = %v, want %v", gotPregap, tt.wantPregap)
			}
		})
	}
}

func TestListSplitFLACs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"02 - Second.flac", "01 - First.FLAC", "cover.jpg", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.flac"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := listSplitFLACs(dir)
	if err != nil {
		t.Fatalf("listSplitFLACs: %v", err)
	}
	want := []string{"01 - First.FLAC", "02 - Second.flac"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listSplitFLACs() = %v, want %v", got, want)
	}
}

func TestListSplitFLACs_MissingDir(t *testing.T) {
	if _, err := listSplitFLACs(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("listSplitFLACs() error = nil, want error for missing dir")
	}
}

// TestFinishSplit exercises the full tag/pregap/cover orchestration against
// fake cueprint/metaflac binaries on PATH, verifying: the pregap file is
// excluded from tagging and then removed, TRACKTOTAL reflects only the real
// (non-pregap) track count, the cover is copied, and result_files excludes
// both the pregap file and the cover.
func TestFinishSplit(t *testing.T) {
	sourceDir := t.TempDir()
	outDir := t.TempDir()

	for _, name := range []string{"00 - pregap.flac", "01 - First.flac", "02 - Second.flac"} {
		if err := os.WriteFile(filepath.Join(outDir, name), []byte("fake"), 0o644); err != nil {
			t.Fatalf("seed output file %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "cover.jpg"), []byte("img"), 0o644); err != nil {
		t.Fatalf("seed cover: %v", err)
	}

	toolDir := t.TempDir()
	writeFakeTool(t, toolDir, "cueprint", "echo tagvalue\n")
	metaflacLog := filepath.Join(t.TempDir(), "metaflac.log")
	writeFakeTool(t, toolDir, "metaflac", `echo "$@" >> "`+metaflacLog+"\"\n")
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	utf8Cue := filepath.Join(sourceDir, "album.cue")
	if err := os.WriteFile(utf8Cue, []byte("dummy"), 0o644); err != nil {
		t.Fatalf("write cue: %v", err)
	}

	album := cue.Album{Genre: "Rock", Date: "2020", Tracks: []cue.Track{{Number: 1}, {Number: 2}}}

	var progressCalls []string
	result, err := finishSplit(context.Background(), utf8Cue, album, sourceDir, outDir, 2, 4, func(current, total int, detail string) {
		progressCalls = append(progressCalls, detail)
	})
	if err != nil {
		t.Fatalf("finishSplit: %v", err)
	}

	want := []string{"01 - First.flac", "02 - Second.flac"}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("result = %v, want %v", result, want)
	}
	if len(progressCalls) == 0 {
		t.Fatal("finishSplit reported no progress")
	}

	if _, statErr := os.Stat(filepath.Join(outDir, "00 - pregap.flac")); !os.IsNotExist(statErr) {
		t.Fatal("pregap file was not removed")
	}
	if _, statErr := os.Stat(filepath.Join(outDir, "cover.jpg")); statErr != nil {
		t.Fatalf("cover was not copied: %v", statErr)
	}

	logData, err := os.ReadFile(metaflacLog)
	if err != nil {
		t.Fatalf("read metaflac log: %v", err)
	}
	log := string(logData)
	if got, want := strings.Count(log, "--set-tag=TRACKTOTAL=2"), 2; got != want {
		t.Fatalf("metaflac log has %d TRACKTOTAL=2 set-tag calls, want %d:\n%s", got, want, log)
	}
	if strings.Contains(log, "pregap") {
		t.Fatalf("pregap file must not be passed to metaflac:\n%s", log)
	}
}

// seedFinishSplit writes the standard three-file split output (one pregap +
// two real tracks) into a fresh outDir, puts no-op cueprint/metaflac fakes
// on PATH, and returns the source dir, the cue path and the out dir.
func seedFinishSplit(t *testing.T) (sourceDir, utf8Cue, outDir string) {
	t.Helper()
	sourceDir = t.TempDir()
	outDir = t.TempDir()

	for _, name := range []string{"00 - pregap.flac", "01 - First.flac", "02 - Second.flac"} {
		if err := os.WriteFile(filepath.Join(outDir, name), []byte("fake"), 0o644); err != nil {
			t.Fatalf("seed output file %s: %v", name, err)
		}
	}

	toolDir := t.TempDir()
	writeFakeTool(t, toolDir, "cueprint", "echo tagvalue\n")
	writeFakeTool(t, toolDir, "metaflac", "exit 0\n")
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	utf8Cue = filepath.Join(sourceDir, "album.cue")
	if err := os.WriteFile(utf8Cue, []byte("dummy"), 0o644); err != nil {
		t.Fatalf("write cue: %v", err)
	}
	return sourceDir, utf8Cue, outDir
}

// The pregap file is discarded audio, but a failure to remove it still fails
// the job: leaving it behind would make the output directory look like it
// holds an extra track, which is exactly what CheckOutputStatus counts.
func TestFinishSplit_PregapRemovalFails(t *testing.T) {
	sourceDir, utf8Cue, outDir := seedFinishSplit(t)

	// A read-only directory permits reading and tagging its files but not
	// unlinking them. Restored on cleanup so t.TempDir can remove the tree.
	if err := os.Chmod(outDir, 0o555); err != nil {
		t.Fatalf("chmod outDir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(outDir, 0o755) })

	album := cue.Album{Tracks: []cue.Track{{Number: 1}, {Number: 2}}}
	_, err := finishSplit(context.Background(), utf8Cue, album, sourceDir, outDir, 2, 4, nil)
	if err == nil || !strings.Contains(err.Error(), "remove pregap file") {
		t.Fatalf("finishSplit() error = %v, want remove-pregap-file failure", err)
	}
}

// A cover that is discovered but cannot be copied fails the job rather than
// completing a split that silently lost its artwork.
func TestFinishSplit_CoverCopyFails(t *testing.T) {
	sourceDir, utf8Cue, outDir := seedFinishSplit(t)

	if err := os.WriteFile(filepath.Join(sourceDir, "cover.jpg"), []byte("img"), 0o644); err != nil {
		t.Fatalf("seed cover: %v", err)
	}
	// The copy destination already exists as a directory, so writing the
	// cover's bytes to it fails.
	if err := os.Mkdir(filepath.Join(outDir, "cover.jpg"), 0o755); err != nil {
		t.Fatalf("seed cover destination dir: %v", err)
	}

	album := cue.Album{Tracks: []cue.Track{{Number: 1}, {Number: 2}}}
	_, err := finishSplit(context.Background(), utf8Cue, album, sourceDir, outDir, 2, 4, nil)
	if err == nil || !strings.Contains(err.Error(), "copy cover") {
		t.Fatalf("finishSplit() error = %v, want copy-cover failure", err)
	}
}

func TestFinishSplit_MissingOutDir(t *testing.T) {
	sourceDir := t.TempDir()
	_, err := finishSplit(context.Background(), filepath.Join(sourceDir, "album.cue"), cue.Album{}, sourceDir, filepath.Join(sourceDir, "missing"), 0, 0, nil)
	if err == nil {
		t.Fatal("finishSplit() error = nil, want error for missing outDir")
	}
}
