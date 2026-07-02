package split

import (
	"strings"
	"testing"
)

func TestParseShnsplitLine(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantStep      bool
		wantTrackName string
	}{
		{
			name:          "progress line with track name",
			line:          "Splitting [Full Album.flac] --> [01 - Track One.flac] : 100% OK",
			wantStep:      true,
			wantTrackName: "01 - Track One.flac",
		},
		{
			name:          "pregap progress line",
			line:          "Splitting [Full Album.flac] --> [00 - pregap.flac] : 100% OK",
			wantStep:      true,
			wantTrackName: "00 - pregap.flac",
		},
		{
			name:     "OK without arrow still counts",
			line:     "  cksum: OK",
			wantStep: true,
		},
		{
			name:     "no-match diagnostic line",
			line:     "shntool: creating output directory",
			wantStep: false,
		},
		{
			name:     "empty line",
			line:     "",
			wantStep: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseShnsplitLine(tt.line)
			if got.isProgressStep != tt.wantStep {
				t.Fatalf("isProgressStep = %v, want %v", got.isProgressStep, tt.wantStep)
			}
			if got.trackName != tt.wantTrackName {
				t.Fatalf("trackName = %q, want %q", got.trackName, tt.wantTrackName)
			}
		})
	}
}

func TestStreamShnsplitProgress_CapsAtTrackCount(t *testing.T) {
	// Real shnsplit output also emits a progress line for the discarded
	// pregap track ("00 - ..."), so 3 progress lines arrive for a 2-track
	// album. The cap must hold the reported current at trackCount.
	stderr := strings.Join([]string{
		"Splitting [Full Album.flac] --> [00 - pregap.flac] : 100% OK",
		"Splitting [Full Album.flac] --> [01 - Track One.flac] : 100% OK",
		"Splitting [Full Album.flac] --> [02 - Track Two.flac] : 100% OK",
	}, "\n")

	const trackCount = 2
	var seen []splitStep
	lines, err := streamShnsplitProgress(strings.NewReader(stderr), trackCount, func(step splitStep) {
		seen = append(seen, step)
	})
	if err != nil {
		t.Fatalf("streamShnsplitProgress: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3", len(lines))
	}
	if len(seen) != 3 {
		t.Fatalf("len(seen) = %d, want 3", len(seen))
	}

	for i, step := range seen {
		if step.current > trackCount {
			t.Fatalf("step %d: current = %d, want <= %d", i, step.current, trackCount)
		}
	}
	if want := trackCount; seen[len(seen)-1].current != want {
		t.Fatalf("final current = %d, want %d", seen[len(seen)-1].current, want)
	}
	if seen[0].detail != "00 - pregap.flac" {
		t.Fatalf("first detail = %q, want pregap track name", seen[0].detail)
	}
}

func TestStreamShnsplitProgress_NoMatchLines(t *testing.T) {
	stderr := "shntool: creating output directory\nsome other diagnostic\n"

	var seen []splitStep
	lines, err := streamShnsplitProgress(strings.NewReader(stderr), 5, func(step splitStep) {
		seen = append(seen, step)
	})
	if err != nil {
		t.Fatalf("streamShnsplitProgress: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}
	if len(seen) != 0 {
		t.Fatalf("len(seen) = %d, want 0", len(seen))
	}
}

func TestStreamShnsplitProgress_NilCallback(t *testing.T) {
	stderr := "Splitting [x] --> [01 - Track.flac] : 100% OK\n"
	if _, err := streamShnsplitProgress(strings.NewReader(stderr), 1, nil); err != nil {
		t.Fatalf("streamShnsplitProgress with nil callback: %v", err)
	}
}
