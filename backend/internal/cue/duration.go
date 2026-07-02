package cue

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// wavHeaderReadLimit bounds how much of a WAV file TotalSeconds reads: only
// the RIFF chunk headers are needed, never the audio samples themselves.
const wavHeaderReadLimit = 1 << 16

// TotalSeconds returns the total duration, in seconds, of the audio file at
// path. FLAC files are read via `metaflac --show-total-samples
// --show-sample-rate`; WAV files are read via a cheap RIFF-header scan with
// no audio decode. Errors are non-fatal: callers (and the frontend) degrade
// gracefully when duration is unknown, treating 0 as "unknown".
func TotalSeconds(path string) (float64, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".flac":
		return flacTotalSeconds(path)
	case ".wav":
		return wavTotalSeconds(path)
	default:
		return 0, fmt.Errorf("cue: unsupported audio extension %q", filepath.Ext(path))
	}
}

func flacTotalSeconds(path string) (float64, error) {
	out, err := exec.Command("metaflac", "--show-total-samples", "--show-sample-rate", path).Output()
	if err != nil {
		return 0, fmt.Errorf("cue: metaflac: %w", err)
	}
	return parseMetaflacDuration(string(out))
}

// parseMetaflacDuration parses the stdout of `metaflac --show-total-samples
// --show-sample-rate` (total sample count and sample rate, one per line)
// into a duration in seconds.
func parseMetaflacDuration(stdout string) (float64, error) {
	fields := strings.Fields(stdout)
	if len(fields) < 2 {
		return 0, fmt.Errorf("cue: unexpected metaflac output %q", stdout)
	}

	samples, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cue: parse total-samples: %w", err)
	}
	rate, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cue: parse sample-rate: %w", err)
	}
	if rate == 0 {
		return 0, fmt.Errorf("cue: metaflac sample rate is 0")
	}

	return float64(samples) / float64(rate), nil
}

func wavTotalSeconds(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("cue: open wav: %w", err)
	}
	defer f.Close()

	header := make([]byte, wavHeaderReadLimit)
	n, err := io.ReadFull(f, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return 0, fmt.Errorf("cue: read wav: %w", err)
	}

	return parseWavDuration(header[:n])
}

// parseWavDuration reads a RIFF/WAVE header's "fmt " and "data" chunks to
// compute duration without decoding audio: seconds = dataChunkSize /
// byteRate. It never reads audio sample bytes, only chunk headers.
func parseWavDuration(header []byte) (float64, error) {
	if len(header) < 12 || string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return 0, fmt.Errorf("cue: not a RIFF/WAVE file")
	}

	var byteRate, dataSize uint32
	haveFmt, haveData := false, false

	pos := 12
	for pos+8 <= len(header) {
		chunkID := string(header[pos : pos+4])
		chunkSize := binary.LittleEndian.Uint32(header[pos+4 : pos+8])
		chunkStart := pos + 8

		switch chunkID {
		case "fmt ":
			if chunkStart+12 > len(header) {
				return 0, fmt.Errorf("cue: truncated wav fmt chunk")
			}
			byteRate = binary.LittleEndian.Uint32(header[chunkStart+8 : chunkStart+12])
			haveFmt = true
		case "data":
			dataSize = chunkSize
			haveData = true
		}

		if haveFmt && haveData {
			break
		}

		advance := int(chunkSize)
		if advance%2 == 1 {
			advance++ // RIFF chunks are word-aligned
		}
		pos = chunkStart + advance
	}

	if !haveFmt || !haveData {
		return 0, fmt.Errorf("cue: wav missing fmt or data chunk")
	}
	if byteRate == 0 {
		return 0, fmt.Errorf("cue: wav byte rate is 0")
	}

	return float64(dataSize) / float64(byteRate), nil
}
