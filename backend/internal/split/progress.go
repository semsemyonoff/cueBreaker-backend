package split

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// maxShnsplitLine bounds the longest stderr line streamed from shnsplit,
// generous enough for any real track or path name.
const maxShnsplitLine = 1 << 20

var shnsplitTrackRe = regexp.MustCompile(`-->\s+\[(.+?)\]`)

// shnsplitEvent is the parsed result of a single shnsplit stderr line.
type shnsplitEvent struct {
	trackName      string
	isProgressStep bool
}

// parseShnsplitLine mirrors app.py's shnsplit stderr parsing: a line
// advances split progress if it contains "OK" or "-->" — shnsplit emits a
// line like `Splitting [file] --> [output] : 100% OK` per track, including
// the discarded pregap track ("00 - ...").
func parseShnsplitLine(line string) shnsplitEvent {
	if !strings.Contains(line, "OK") && !strings.Contains(line, "-->") {
		return shnsplitEvent{}
	}
	event := shnsplitEvent{isProgressStep: true}
	if m := shnsplitTrackRe.FindStringSubmatch(line); m != nil {
		event.trackName = m[1]
	}
	return event
}

// splitStep is one parsed progress-worthy shnsplit line: current is the
// split-half progress counter, capped at trackCount so the pregap line
// (which also matches parseShnsplitLine) can never push it past the real
// track count.
type splitStep struct {
	current int
	detail  string
}

// streamShnsplitProgress reads shnsplit stderr line by line, translating
// each progress line into a splitStep passed to onStep, and returns every
// line read (so a caller can fold them into an error message if the
// process itself fails). onStep may be nil.
func streamShnsplitProgress(r io.Reader, trackCount int, onStep func(splitStep)) ([]string, error) {
	var lines []string
	stepsSeen := 0

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxShnsplitLine)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)

		event := parseShnsplitLine(line)
		if !event.isProgressStep {
			continue
		}

		stepsSeen++
		current := min(stepsSeen, trackCount)
		detail := event.trackName
		if detail == "" {
			detail = fmt.Sprintf("track %d", stepsSeen)
		}
		if onStep != nil {
			onStep(splitStep{current: current, detail: detail})
		}
	}

	return lines, scanner.Err()
}
