package cue

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
)

// encodingNames lists the encodings tried, in order, when auto-detecting a
// CUE sheet's character set. It mirrors the Python app's ENCODINGS list.
var encodingNames = []string{"utf-8-sig", "utf-8", "cp1251", "cp1252", "shift_jis", "euc-kr", "latin-1"}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// namedEncoding maps an encoding name to its x/text codec. utf-8 and
// utf-8-sig are handled separately since they use the standard library's
// UTF-8 validation rather than an x/text charmap.
func namedEncoding(name string) encoding.Encoding {
	switch name {
	case "cp1251":
		return charmap.Windows1251
	case "cp1252":
		return charmap.Windows1252
	case "shift_jis":
		return japanese.ShiftJIS
	case "euc-kr":
		return korean.EUCKR
	case "latin-1":
		return charmap.ISO8859_1
	default:
		return nil
	}
}

// decodeAs decodes raw bytes as the named encoding, returning an error if
// the bytes are not valid in that encoding.
func decodeAs(raw []byte, name string) (string, error) {
	switch name {
	case "utf-8-sig":
		stripped := bytes.TrimPrefix(raw, utf8BOM)
		if !utf8.Valid(stripped) {
			return "", fmt.Errorf("invalid utf-8")
		}
		return string(stripped), nil
	case "utf-8":
		if !utf8.Valid(raw) {
			return "", fmt.Errorf("invalid utf-8")
		}
		return string(raw), nil
	default:
		enc := namedEncoding(name)
		if enc == nil {
			return "", fmt.Errorf("unknown encoding %q", name)
		}
		out, err := enc.NewDecoder().Bytes(raw)
		if err != nil {
			return "", err
		}
		// x/text's decoders substitute unmappable bytes with the Unicode
		// replacement rune instead of erroring; treat that as a failed
		// decode, matching Python's strict codecs (which raise on them).
		if strings.ContainsRune(string(out), utf8.RuneError) {
			return "", fmt.Errorf("encoding %q: unmappable byte", name)
		}
		return string(out), nil
	}
}

// detectDecode tries each encoding in encodingNames in order, returning the
// first decode that contains both "TRACK" and "INDEX". If none match, it
// falls back to latin-1 (which never fails to decode).
func detectDecode(raw []byte) (text string, enc string, matched bool) {
	for _, name := range encodingNames {
		decoded, err := decodeAs(raw, name)
		if err != nil {
			continue
		}
		if strings.Contains(decoded, "TRACK") && strings.Contains(decoded, "INDEX") {
			return decoded, name, true
		}
	}
	decoded, err := decodeAs(raw, "latin-1")
	if err != nil {
		// latin-1 maps every byte to a rune and never fails; this is
		// unreachable but kept as a defensive fallback.
		return string(raw), "latin-1", false
	}
	return decoded, "latin-1", false
}

// ReadCUE reads a CUE file, auto-detecting its encoding, and returns its
// contents decoded as a UTF-8 Go string.
func ReadCUE(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read cue %s: %w", path, err)
	}

	text, enc, matched := detectDecode(raw)
	if matched {
		slog.Info("cue encoding detected", "path", path, "encoding", enc)
	} else {
		slog.Warn("cue encoding: no encoding matched, falling back to latin-1", "path", path)
	}
	return text, nil
}

// MakeUTF8Cue writes a temporary UTF-8 copy of the CUE file at path and
// returns its path. The caller is responsible for removing the temp file.
func MakeUTF8Cue(path string) (string, error) {
	text, err := ReadCUE(path)
	if err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp("", "*.cue")
	if err != nil {
		return "", fmt.Errorf("create temp cue: %w", err)
	}
	defer tmp.Close()

	if _, err := tmp.WriteString(text); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write temp cue: %w", err)
	}

	return tmp.Name(), nil
}
