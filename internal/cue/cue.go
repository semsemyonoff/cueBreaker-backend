package cue

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Track is a single CUE track entry.
type Track struct {
	Number       int     `json:"number"`
	Title        string  `json:"title"`
	Performer    string  `json:"performer"`
	Index        string  `json:"index"`
	StartSeconds float64 `json:"start_seconds"`
}

// Album is the parsed contents of a CUE sheet: album-level metadata plus
// its tracks, in CUE order.
type Album struct {
	Performer string  `json:"performer"`
	Title     string  `json:"title"`
	File      string  `json:"file"`
	Genre     string  `json:"genre"`
	Date      string  `json:"date"`
	Tracks    []Track `json:"tracks"`
}

var (
	performerRe      = regexp.MustCompile(`(?m)^PERFORMER\s+"(.+?)"`)
	titleRe          = regexp.MustCompile(`(?m)^TITLE\s+"(.+?)"`)
	fileRe           = regexp.MustCompile(`(?m)^FILE\s+"(.+?)"`)
	genreRe          = regexp.MustCompile(`(?m)^REM\s+GENRE\s+"?(.+?)"?\s*$`)
	dateRe           = regexp.MustCompile(`(?m)^REM\s+DATE\s+(\S+)`)
	trackHeaderRe    = regexp.MustCompile(`(?m)^\s*TRACK\s+(\d+)\s+AUDIO`)
	trackTitleRe     = regexp.MustCompile(`TITLE\s+"(.+?)"`)
	trackPerformerRe = regexp.MustCompile(`PERFORMER\s+"(.+?)"`)
	trackIndexRe     = regexp.MustCompile(`INDEX\s+01\s+(\d+:\d+:\d+)`)
)

// Parse reads and parses a CUE file, auto-detecting its encoding.
func Parse(path string) (Album, error) {
	content, err := ReadCUE(path)
	if err != nil {
		return Album{}, err
	}
	return ParseText(content), nil
}

// ParseText parses already-decoded CUE sheet text into album metadata and
// its tracks. It never errors: missing fields are left as zero values,
// mirroring the Python parser it replaces.
func ParseText(content string) Album {
	album := Album{
		Performer: firstMatch(performerRe, content),
		Title:     firstMatch(titleRe, content),
		File:      firstMatch(fileRe, content),
		Genre:     firstMatch(genreRe, content),
		Date:      firstMatch(dateRe, content),
	}

	headers := trackHeaderRe.FindAllStringSubmatchIndex(content, -1)
	for i, h := range headers {
		num, err := strconv.Atoi(content[h[2]:h[3]])
		if err != nil {
			continue
		}

		blockEnd := len(content)
		if i+1 < len(headers) {
			blockEnd = headers[i+1][0]
		}
		block := content[h[1]:blockEnd]

		track := Track{Number: num, Performer: album.Performer}
		if title := firstMatch(trackTitleRe, block); title != "" {
			track.Title = title
		}
		if performer := firstMatch(trackPerformerRe, block); performer != "" {
			track.Performer = performer
		}
		if index := firstMatch(trackIndexRe, block); index != "" {
			track.Index = index
			track.StartSeconds = indexToSeconds(index)
		}

		album.Tracks = append(album.Tracks, track)
	}

	return album
}

func firstMatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// indexToSeconds converts an INDEX 01 timestamp (MM:SS:FF, 75 frames per
// second) to a fractional second offset. Malformed input yields 0.
func indexToSeconds(index string) float64 {
	parts := strings.Split(index, ":")
	if len(parts) != 3 {
		return 0
	}
	minutes, err1 := strconv.ParseFloat(parts[0], 64)
	seconds, err2 := strconv.ParseFloat(parts[1], 64)
	frames, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0
	}
	return minutes*60 + seconds + frames/75
}

// fileReferences returns every FILE "..." reference in a CUE sheet, in
// document order. A single-file CUE (whole album in one source file) has
// exactly one; a multi-file CUE (already split, one FILE per track) has
// more than one.
func fileReferences(content string) []string {
	matches := fileRe.FindAllStringSubmatch(content, -1)
	refs := make([]string, len(matches))
	for i, m := range matches {
		refs[i] = m[1]
	}
	return refs
}

// Sentinel errors identifying why CheckSourceFLAC rejected a CUE sheet.
// Each is wrapped with context via fmt.Errorf("%w: ...", ...), so callers
// compare with errors.Is rather than matching message text.
var (
	// ErrCUEUnreadable means the CUE sheet could not be read or decoded.
	ErrCUEUnreadable = errors.New("cue unreadable")
	// ErrNoFileReference means the CUE sheet has no FILE reference.
	ErrNoFileReference = errors.New("no FILE reference")
	// ErrMultiFileReference means the CUE sheet has more than one FILE
	// reference — the album is already split into per-track files.
	ErrMultiFileReference = errors.New("multi-file cue")
	// ErrNotFLACOrWAV means the CUE's single FILE reference does not name
	// a .flac/.wav file.
	ErrNotFLACOrWAV = errors.New("source is not FLAC/WAV")
	// ErrSourceMissing means the referenced source file does not exist,
	// or its real path (after resolving symlinks) is not contained in dir.
	ErrSourceMissing = errors.New("source file missing")
)

// CheckSourceFLAC reports why cuePath is not a single-file CUE whose FILE
// reference points at an existing FLAC/WAV in dir (i.e. an unsplit album),
// or nil when it is. The referenced file must stay under dir after
// resolving symlinks — the same containment SourceFLAC enforces — so scan
// eligibility never accepts an album whose FILE "../x.flac" (or a symlinked
// source) later preview/split resolution would reject.
func CheckSourceFLAC(cuePath, dir string) error {
	content, err := ReadCUE(cuePath)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrCUEUnreadable, filepath.Base(cuePath), err)
	}

	refs := fileReferences(content)
	if len(refs) == 0 {
		return fmt.Errorf("%w in %s", ErrNoFileReference, filepath.Base(cuePath))
	}
	if len(refs) > 1 {
		return fmt.Errorf("%w (already split): %s", ErrMultiFileReference, filepath.Base(cuePath))
	}

	ref := refs[0]
	lower := strings.ToLower(ref)
	if !strings.HasSuffix(lower, ".flac") && !strings.HasSuffix(lower, ".wav") {
		return fmt.Errorf("%w: %s", ErrNotFLACOrWAV, ref)
	}

	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrSourceMissing, ref)
	}
	if _, ok := containedFile(dir, realDir, ref); !ok {
		return fmt.Errorf("%w: %s", ErrSourceMissing, ref)
	}

	return nil
}

// SourceFLAC resolves the source audio file for album within dir: the
// CUE's FILE reference if present and it exists, else the first .flac/.wav
// file in dir. The candidate is rejected (ok=false) unless its real path
// (after resolving symlinks) stays under dir's real path, which blocks both
// FILE ".." traversal and symlink escapes.
func SourceFLAC(album Album, dir string) (path string, ok bool) {
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", false
	}

	if album.File != "" {
		if p, ok := containedFile(dir, realDir, album.File); ok {
			return p, true
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		lower := strings.ToLower(e.Name())
		if !strings.HasSuffix(lower, ".flac") && !strings.HasSuffix(lower, ".wav") {
			continue
		}
		if p, ok := containedFile(dir, realDir, e.Name()); ok {
			return p, true
		}
	}

	return "", false
}

// containedFile joins dir and name, and returns that path only if it names
// an existing file whose real path (after resolving symlinks) stays under
// realDir.
func containedFile(dir, realDir, name string) (string, bool) {
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
