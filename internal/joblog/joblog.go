package joblog

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Level classifies a log entry's severity.
type Level string

// Log levels, ordered from least to most severe.
const (
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Entry is a single, immutable log line with its position in the buffer.
type Entry struct {
	Seq   int       `json:"seq"`
	Time  time.Time `json:"time"`
	Level Level     `json:"level"`
	Text  string    `json:"text"`
}

// DefaultCapacity is the entry count New falls back to when given a
// non-positive capacity.
const DefaultCapacity = 500

// Buffer is a bounded, monotonically-sequenced log ring, safe for concurrent
// use. The zero value is not ready to use; construct one with New. A nil
// *Buffer is safe to call Add and Since on: Add becomes a no-op and Since
// returns no entries, so a State predating a job's start need not special-case
// a missing log.
type Buffer struct {
	mu       sync.Mutex
	capacity int
	entries  []Entry
	nextSeq  int
}

// New returns a Buffer retaining at most capacity entries. A non-positive
// capacity falls back to DefaultCapacity.
func New(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Buffer{capacity: capacity}
}

// Add formats and appends a log entry, assigning it the next monotonic
// sequence number and evicting the oldest entry once the buffer is over
// capacity. Control characters — including a stray NUL read off disk from a
// filename — are stripped from the formatted text before it is stored.
func (b *Buffer) Add(level Level, format string, args ...any) {
	if b == nil {
		return
	}
	text := stripControl(fmt.Sprintf(format, args...))

	b.mu.Lock()
	defer b.mu.Unlock()

	entry := Entry{
		Seq:   b.nextSeq,
		Time:  time.Now(),
		Level: level,
		Text:  text,
	}
	b.nextSeq++

	b.entries = append(b.entries, entry)
	if len(b.entries) > b.capacity {
		copy(b.entries, b.entries[1:])
		b.entries = b.entries[:b.capacity]
	}
}

// Since returns every retained entry with Seq >= seq, oldest first, plus the
// sequence number the caller should pass to the next call. next is computed
// by the buffer rather than left for the caller to derive from max(seq)+1: a
// call that returns no entries still advances the caller's cursor correctly.
// It is safe to call on a nil Buffer, returning no entries and a next cursor
// of 0.
func (b *Buffer) Since(seq int) (entries []Entry, next int) {
	if b == nil {
		return nil, 0
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for _, e := range b.entries {
		if e.Seq >= seq {
			entries = append(entries, e)
		}
	}
	return entries, b.nextSeq
}

func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
