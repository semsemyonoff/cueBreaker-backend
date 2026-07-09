package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindCover_NamedMatchPriority(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "album.jpg"), "x")
	writeFile(t, filepath.Join(dir, "Folder.png"), "x")
	writeFile(t, filepath.Join(dir, "COVER.jpg"), "x")

	path, ok := FindCover(dir)
	if !ok {
		t.Fatalf("FindCover: ok = false, want true")
	}
	if filepath.Base(path) != "COVER.jpg" {
		t.Errorf("FindCover = %q, want COVER.jpg (cover beats folder/album, case-insensitive)", path)
	}
}

func TestFindCover_ExtensionFilter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "cover.txt"), "not an image")
	writeFile(t, filepath.Join(dir, "front.webp"), "x")

	path, ok := FindCover(dir)
	if !ok {
		t.Fatalf("FindCover: ok = false, want true")
	}
	if filepath.Base(path) != "front.webp" {
		t.Errorf("FindCover = %q, want front.webp (cover.txt is not an image)", path)
	}
}

func TestFindCover_FallbackToFirst(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "zzz.jpg"), "x")
	writeFile(t, filepath.Join(dir, "aaa.png"), "x")

	path, ok := FindCover(dir)
	if !ok {
		t.Fatalf("FindCover: ok = false, want true")
	}
	if filepath.Base(path) != "aaa.png" {
		t.Errorf("FindCover = %q, want aaa.png (alphabetically first image, no named match)", path)
	}
}

func TestFindCover_NoneFound(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "album.cue"), "x")
	writeFile(t, filepath.Join(dir, "notes.txt"), "x")

	path, ok := FindCover(dir)
	if ok {
		t.Fatalf("FindCover = %q, ok = true, want false", path)
	}
}

func TestFindCover_SymlinkEscapeRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on windows")
	}

	dir := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.jpg"), "x")

	if err := os.Symlink(filepath.Join(outside, "secret.jpg"), filepath.Join(dir, "cover.jpg")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	path, ok := FindCover(dir)
	if ok {
		t.Fatalf("FindCover = %q, ok = true, want false (symlink escapes dir)", path)
	}
}
