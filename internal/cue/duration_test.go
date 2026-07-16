package cue

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseMetaflacDuration(t *testing.T) {
	tests := []struct {
		name    string
		stdout  string
		want    float64
		wantErr bool
	}{
		{name: "valid", stdout: "44100\n44100\n", want: 1.0},
		{name: "valid fractional", stdout: "22050\n44100\n", want: 0.5},
		{name: "zero rate", stdout: "100\n0\n", wantErr: true},
		{name: "garbage", stdout: "not a number\n", wantErr: true},
		{name: "empty", stdout: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMetaflacDuration(tt.stdout)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseMetaflacDuration(%q) = %v, want error", tt.stdout, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMetaflacDuration(%q) unexpected error: %v", tt.stdout, err)
			}
			if got != tt.want {
				t.Fatalf("parseMetaflacDuration(%q) = %v, want %v", tt.stdout, got, tt.want)
			}
		})
	}
}

// buildWavHeader constructs a minimal valid RIFF/WAVE header (fmt + data
// chunks, no sample bytes) for a given sample rate, channel count, bit
// depth, and data-chunk byte size.
func buildWavHeader(sampleRate, byteRate uint32, dataSize uint32) []byte {
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16) // fmt chunk size
	binary.LittleEndian.PutUint16(header[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(header[22:24], 1)  // mono
	binary.LittleEndian.PutUint32(header[24:28], sampleRate)
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	binary.LittleEndian.PutUint16(header[32:34], 2)  // block align
	binary.LittleEndian.PutUint16(header[34:36], 16) // bits per sample
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)
	return header
}

func TestParseWavDuration(t *testing.T) {
	t.Run("valid header", func(t *testing.T) {
		header := buildWavHeader(44100, 88200, 88200) // 1 second, 16-bit mono @44.1kHz
		got, err := parseWavDuration(header)
		if err != nil {
			t.Fatalf("parseWavDuration() unexpected error: %v", err)
		}
		if got != 1.0 {
			t.Fatalf("parseWavDuration() = %v, want 1.0", got)
		}
	})

	t.Run("header with extra chunk before data", func(t *testing.T) {
		header := buildWavHeader(8000, 16000, 8000) // 0.5s
		listChunk := []byte("LIST")
		listChunk = binary.LittleEndian.AppendUint32(listChunk, 4)
		listChunk = append(listChunk, []byte("INFO")...)
		// splice the LIST chunk in right after the fmt chunk (offset 36)
		withExtra := append(append(append([]byte{}, header[:36]...), listChunk...), header[36:]...)
		got, err := parseWavDuration(withExtra)
		if err != nil {
			t.Fatalf("parseWavDuration() unexpected error: %v", err)
		}
		if got != 0.5 {
			t.Fatalf("parseWavDuration() = %v, want 0.5", got)
		}
	})

	// RIFF chunks are word-aligned: an odd-sized chunk carries a pad byte
	// that its size field does not count. A parser that advanced by the raw
	// size would land one byte short and read a garbage chunk header for the
	// data chunk, so this pins the alignment step.
	t.Run("odd-sized chunk before data is word-aligned", func(t *testing.T) {
		header := buildWavHeader(8000, 16000, 8000) // 0.5s
		oddChunk := []byte("LIST")
		oddChunk = binary.LittleEndian.AppendUint32(oddChunk, 3)
		oddChunk = append(oddChunk, []byte("INF")...) // 3 bytes of payload...
		oddChunk = append(oddChunk, 0)                // ...plus the pad byte

		withOdd := append(append(append([]byte{}, header[:36]...), oddChunk...), header[36:]...)
		got, err := parseWavDuration(withOdd)
		if err != nil {
			t.Fatalf("parseWavDuration() unexpected error: %v", err)
		}
		if got != 0.5 {
			t.Fatalf("parseWavDuration() = %v, want 0.5", got)
		}
	})

	t.Run("truncated fmt chunk", func(t *testing.T) {
		header := buildWavHeader(44100, 88200, 88200)
		// 24 bytes: past the fmt chunk's own header, but short of its byteRate
		// field at chunkStart+8.
		if _, err := parseWavDuration(header[:24]); err == nil {
			t.Fatalf("parseWavDuration(truncated fmt chunk) = nil error, want error")
		}
	})

	t.Run("missing fmt chunk", func(t *testing.T) {
		header := buildWavHeader(44100, 88200, 88200)
		// Rename "fmt " so only the data chunk is recognized.
		copy(header[12:16], "junk")
		if _, err := parseWavDuration(header); err == nil {
			t.Fatalf("parseWavDuration(no fmt chunk) = nil error, want error")
		}
	})

	t.Run("truncated", func(t *testing.T) {
		header := buildWavHeader(44100, 88200, 88200)
		if _, err := parseWavDuration(header[:20]); err == nil {
			t.Fatalf("parseWavDuration(truncated) = nil error, want error")
		}
	})

	t.Run("garbage", func(t *testing.T) {
		if _, err := parseWavDuration([]byte("not a wav file at all")); err == nil {
			t.Fatalf("parseWavDuration(garbage) = nil error, want error")
		}
	})

	t.Run("zero byte rate", func(t *testing.T) {
		header := buildWavHeader(44100, 0, 88200)
		if _, err := parseWavDuration(header); err == nil {
			t.Fatalf("parseWavDuration(zero byte rate) = nil error, want error")
		}
	})

	t.Run("missing data chunk", func(t *testing.T) {
		header := buildWavHeader(44100, 88200, 88200)
		if _, err := parseWavDuration(header[:36]); err == nil {
			t.Fatalf("parseWavDuration(no data chunk) = nil error, want error")
		}
	})
}

func TestTotalSeconds_UnsupportedExtension(t *testing.T) {
	if _, err := TotalSeconds("album.mp3"); err == nil {
		t.Fatalf("TotalSeconds(.mp3) = nil error, want error")
	}
}

func TestTotalSeconds_Wav(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.wav")
	header := buildWavHeader(8000, 16000, 8000) // 0.5s
	if err := os.WriteFile(path, header, 0o644); err != nil {
		t.Fatalf("write wav fixture: %v", err)
	}

	got, err := TotalSeconds(path)
	if err != nil {
		t.Fatalf("TotalSeconds(%q) unexpected error: %v", path, err)
	}
	if got != 0.5 {
		t.Fatalf("TotalSeconds(%q) = %v, want 0.5", path, got)
	}
}

// A FLAC duration read depends on metaflac being installed. When it is not,
// TotalSeconds must surface the error rather than report a bogus duration —
// callers treat an error as "unknown" and degrade.
func TestTotalSeconds_Flac_MetaflacMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "album.flac")
	if err := os.WriteFile(path, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write flac fixture: %v", err)
	}
	// An empty PATH: metaflac cannot be found, so exec fails before it runs.
	t.Setenv("PATH", t.TempDir())

	got, err := TotalSeconds(path)
	if err == nil {
		t.Fatalf("TotalSeconds() = %v, nil error; want an error when metaflac is missing", got)
	}
	if got != 0 {
		t.Fatalf("TotalSeconds() = %v, want 0 alongside the error", got)
	}
}

func TestTotalSeconds_Flac_RealMetaflac(t *testing.T) {
	if _, err := exec.LookPath("metaflac"); err != nil {
		t.Skip("metaflac not available on PATH")
	}

	got, err := TotalSeconds(filepath.Join("..", "..", "testdata", "audio", "tiny.flac"))
	if err != nil {
		t.Fatalf("TotalSeconds() unexpected error: %v", err)
	}
	if got != 0.5 {
		t.Fatalf("TotalSeconds() = %v, want 0.5", got)
	}
}
