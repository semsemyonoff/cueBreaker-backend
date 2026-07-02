package cue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "cue", name)
}

func TestReadCUE_DetectsEncoding(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantSub string
	}{
		{"utf-8", "utf8.cue", "Test Artist"},
		{"utf-8 with BOM", "utf8_bom.cue", "Test Artist"},
		{"cp1251", "cp1251.cue", "Тестовый Артист"},
		{"shift_jis", "shift_jis.cue", "テストアルバム"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, err := ReadCUE(testdataPath(t, tt.file))
			if err != nil {
				t.Fatalf("ReadCUE() returned error: %v", err)
			}
			if !strings.Contains(text, tt.wantSub) {
				t.Errorf("ReadCUE() = %q, want substring %q", text, tt.wantSub)
			}
			if !strings.Contains(text, "TRACK") || !strings.Contains(text, "INDEX") {
				t.Errorf("ReadCUE() = %q, want TRACK and INDEX present", text)
			}
		})
	}
}

func TestDecodeAs_EucKR(t *testing.T) {
	raw, err := os.ReadFile(testdataPath(t, "euc_kr.cue"))
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	text, err := decodeAs(raw, "euc-kr")
	if err != nil {
		t.Fatalf("decodeAs(euc-kr) returned error: %v", err)
	}
	if !strings.Contains(text, "테스트 아티스트") {
		t.Errorf("decodeAs(euc-kr) = %q, want it to contain decoded Hangul text", text)
	}
}

func TestReadCUE_BOMStripped(t *testing.T) {
	text, err := ReadCUE(testdataPath(t, "utf8_bom.cue"))
	if err != nil {
		t.Fatalf("ReadCUE() returned error: %v", err)
	}
	if strings.HasPrefix(text, "\ufeff") {
		t.Errorf("ReadCUE() left a BOM at the start of the text")
	}
	if !strings.HasPrefix(text, "PERFORMER") {
		t.Errorf("ReadCUE() = %q, want it to start with PERFORMER after BOM strip", text[:min(20, len(text))])
	}
}

func TestReadCUE_FallbackToLatin1(t *testing.T) {
	text, err := ReadCUE(testdataPath(t, "fallback_binary.cue"))
	if err != nil {
		t.Fatalf("ReadCUE() returned error: %v", err)
	}
	raw, err := os.ReadFile(testdataPath(t, "fallback_binary.cue"))
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	want, err := decodeAs(raw, "latin-1")
	if err != nil {
		t.Fatalf("decodeAs(latin-1) returned error: %v", err)
	}
	if text != want {
		t.Errorf("ReadCUE() = %q, want latin-1 fallback %q", text, want)
	}
}

func TestReadCUE_MissingFile(t *testing.T) {
	if _, err := ReadCUE(testdataPath(t, "does_not_exist.cue")); err == nil {
		t.Error("ReadCUE() with missing file: expected error, got nil")
	}
}

func TestDetectDecode(t *testing.T) {
	tests := []struct {
		name        string
		raw         []byte
		wantEnc     string
		wantMatched bool
	}{
		{"ascii TRACK+INDEX", []byte("PERFORMER \"x\"\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"), "utf-8-sig", true},
		{"no TRACK or INDEX", []byte{0xFF, 0xFE, 0x00, 0x01}, "latin-1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, enc, matched := detectDecode(tt.raw)
			if enc != tt.wantEnc {
				t.Errorf("detectDecode() enc = %q, want %q", enc, tt.wantEnc)
			}
			if matched != tt.wantMatched {
				t.Errorf("detectDecode() matched = %v, want %v", matched, tt.wantMatched)
			}
		})
	}
}

func TestMakeUTF8Cue(t *testing.T) {
	tmpPath, err := MakeUTF8Cue(testdataPath(t, "cp1251.cue"))
	if err != nil {
		t.Fatalf("MakeUTF8Cue() returned error: %v", err)
	}
	defer os.Remove(tmpPath)

	if filepath.Ext(tmpPath) != ".cue" {
		t.Errorf("MakeUTF8Cue() path = %q, want .cue suffix", tmpPath)
	}

	got, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("failed to read temp cue: %v", err)
	}
	if !strings.Contains(string(got), "Тестовый Артист") {
		t.Errorf("temp cue content = %q, want it to contain decoded Cyrillic text", got)
	}

	want, err := ReadCUE(testdataPath(t, "cp1251.cue"))
	if err != nil {
		t.Fatalf("ReadCUE() returned error: %v", err)
	}
	if string(got) != want {
		t.Errorf("temp cue content = %q, want %q", got, want)
	}
}

func TestMakeUTF8Cue_MissingFile(t *testing.T) {
	if _, err := MakeUTF8Cue(testdataPath(t, "does_not_exist.cue")); err == nil {
		t.Error("MakeUTF8Cue() with missing file: expected error, got nil")
	}
}
