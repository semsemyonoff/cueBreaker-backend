package split

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"git.horn/cueBreaker/backend/internal/cue"
	"git.horn/cueBreaker/backend/internal/scan"
)

// tagTimeout bounds each cueprint/metaflac call, mirroring app.py's
// subprocess timeout=10 for tagging.
const tagTimeout = 10 * time.Second

// pregapPrefix marks the discarded pre-track-1 audio that shnsplit always
// emits alongside the real tracks (shntool's "00 - ..." naming convention).
const pregapPrefix = "00 -"

// trackTagFields are the resolved values used to build one track's tag
// set. Cueprint-derived fields may be empty (cueprint returned nothing or
// failed), in which case that tag is omitted; TrackTotal always comes from
// the real (non-pregap) split output count, not cueprint.
type trackTagFields struct {
	Title       string
	Artist      string
	TrackNumber string
	Album       string
	AlbumArtist string
	Genre       string
	Date        string
	TrackTotal  int
}

// buildTags assembles the metaflac --set-tag map for one track. Empty
// cueprint-derived fields are omitted entirely (matching app.py's
// "only set if non-empty" behavior); GENRE/DATE come from the CUE parse
// (cueprint has no such fields) and TRACKTOTAL is always present.
func buildTags(f trackTagFields) map[string]string {
	tags := make(map[string]string, 7)
	setIfNonEmpty(tags, "TITLE", f.Title)
	setIfNonEmpty(tags, "ARTIST", f.Artist)
	setIfNonEmpty(tags, "TRACKNUMBER", f.TrackNumber)
	setIfNonEmpty(tags, "ALBUM", f.Album)
	setIfNonEmpty(tags, "ALBUMARTIST", f.AlbumArtist)
	setIfNonEmpty(tags, "GENRE", f.Genre)
	setIfNonEmpty(tags, "DATE", f.Date)
	tags["TRACKTOTAL"] = strconv.Itoa(f.TrackTotal)
	return tags
}

func setIfNonEmpty(tags map[string]string, key, value string) {
	if value != "" {
		tags[key] = value
	}
}

// isPregapFile reports whether name is shnsplit's discarded pregap output
// ("00 - ...").
func isPregapFile(name string) bool {
	return strings.HasPrefix(name, pregapPrefix)
}

// partitionPregap splits split-output file names into real tracks and the
// discarded pregap file(s), preserving input order.
func partitionPregap(names []string) (real, pregap []string) {
	for _, name := range names {
		if isPregapFile(name) {
			pregap = append(pregap, name)
		} else {
			real = append(real, name)
		}
	}
	return real, pregap
}

// listSplitFLACs returns the base names of every *.flac file directly in
// dir, sorted (mirrors app.py's sorted(glob.glob(os.path.join(out_dir,
// "*.flac")))).
func listSplitFLACs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".flac") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// finishSplit tags every real (non-pregap) track already written to
// outDir, removes the discarded pregap file(s), copies a discovered cover
// from sourceDir into outDir, and returns the sorted list of resulting
// FLAC file names (result_files). trackCount/totalSteps are the same
// values Run used to size the split half of the progress bar; the tagging
// half continues from trackCount up to totalSteps.
func finishSplit(ctx context.Context, utf8Cue string, album cue.Album, sourceDir, outDir string, trackCount, totalSteps int, r reporter) ([]string, error) {
	names, err := listSplitFLACs(outDir)
	if err != nil {
		return nil, fmt.Errorf("split: list output files: %w", err)
	}
	realNames, pregapNames := partitionPregap(names)

	albumTitle := cueprintAlbumField(ctx, utf8Cue, "%T")
	albumArtist := cueprintAlbumField(ctx, utf8Cue, "%P")

	for i, name := range realNames {
		trackNum := i + 1
		r.step(trackCount+trackNum, totalSteps, "Tagging: "+name)

		fields := trackTagFields{
			Title:       cueprintTrackField(ctx, utf8Cue, trackNum, "%t"),
			Artist:      cueprintTrackField(ctx, utf8Cue, trackNum, "%p"),
			TrackNumber: cueprintTrackField(ctx, utf8Cue, trackNum, "%n"),
			Album:       albumTitle,
			AlbumArtist: albumArtist,
			Genre:       album.Genre,
			Date:        album.Date,
			TrackTotal:  len(realNames),
		}
		applyMetaflacTags(ctx, filepath.Join(outDir, name), buildTags(fields))
		r.info("tagged %s: %s", trackFraction(trackNum, len(realNames)), name)
	}

	r.step(totalSteps, totalSteps, "Copying cover...")
	for _, name := range pregapNames {
		if err := os.Remove(filepath.Join(outDir, name)); err != nil {
			return nil, fmt.Errorf("split: remove pregap file: %w", err)
		}
		r.warn("removed pregap file: %s", name)
	}
	if coverPath, ok := scan.FindCover(sourceDir); ok {
		dst := filepath.Join(outDir, filepath.Base(coverPath))
		if err := copyFile(coverPath, dst); err != nil {
			return nil, fmt.Errorf("split: copy cover: %w", err)
		}
		r.info("cover copied: %s", filepath.Base(coverPath))
	} else {
		r.warn("no cover found")
	}

	result, err := listSplitFLACs(outDir)
	if err != nil {
		return nil, fmt.Errorf("split: list result files: %w", err)
	}
	return result, nil
}

// cueprintTrackField runs cueprint for a single track's field (e.g. %t for
// title), returning "" if the tool fails or prints nothing — a missing tag
// is not fatal to the split, matching app.py.
func cueprintTrackField(ctx context.Context, utf8Cue string, trackNum int, format string) string {
	out, err := runContext(ctx, tagTimeout, "cueprint", "-n", strconv.Itoa(trackNum), "-t", format, utf8Cue)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// cueprintAlbumField runs cueprint for an album-level field (e.g. %T for
// album title), returning "" if the tool fails or prints nothing.
func cueprintAlbumField(ctx context.Context, utf8Cue, format string) string {
	out, err := runContext(ctx, tagTimeout, "cueprint", "-d", format, utf8Cue)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// applyMetaflacTags clears then rewrites flacPath's tags. Failures are
// best-effort: app.py logs a warning and moves on to the next track rather
// than failing the whole split over one track's tags, so this does not
// return an error.
func applyMetaflacTags(ctx context.Context, flacPath string, tags map[string]string) {
	_, _ = runContext(ctx, tagTimeout, "metaflac", "--remove-all-tags", flacPath)

	args := make([]string, 0, len(tags)+1)
	for k, v := range tags {
		args = append(args, fmt.Sprintf("--set-tag=%s=%s", k, v))
	}
	args = append(args, flacPath)
	_, _ = runContext(ctx, tagTimeout, "metaflac", args...)
}

// copyFile copies src's contents to dst, creating or truncating dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
