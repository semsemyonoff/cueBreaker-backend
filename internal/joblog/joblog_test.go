package joblog

import (
	"sync"
	"testing"
)

func TestBuffer_AddSinceRoundTrip(t *testing.T) {
	b := New(10)
	b.Add(LevelInfo, "line %d", 1)
	b.Add(LevelWarn, "line %d", 2)
	b.Add(LevelError, "line %d", 3)

	entries, next := b.Since(0)
	if len(entries) != 3 {
		t.Fatalf("Since(0) returned %d entries, want 3", len(entries))
	}
	wantLevels := []Level{LevelInfo, LevelWarn, LevelError}
	for i, e := range entries {
		if e.Seq != i {
			t.Errorf("entries[%d].Seq = %d, want %d", i, e.Seq, i)
		}
		if e.Level != wantLevels[i] {
			t.Errorf("entries[%d].Level = %q, want %q", i, e.Level, wantLevels[i])
		}
		wantText := "line " + string(rune('1'+i))
		if e.Text != wantText {
			t.Errorf("entries[%d].Text = %q, want %q", i, e.Text, wantText)
		}
	}
	if next != 3 {
		t.Errorf("next = %d, want 3", next)
	}
}

func TestBuffer_Eviction(t *testing.T) {
	b := New(3)
	for i := range 5 {
		b.Add(LevelInfo, "entry %d", i)
	}

	entries, next := b.Since(0)
	if len(entries) != 3 {
		t.Fatalf("Since(0) returned %d entries, want 3", len(entries))
	}
	// Oldest two (seq 0, 1) evicted; retained tail is seq 2,3,4, never renumbered.
	wantSeqs := []int{2, 3, 4}
	for i, e := range entries {
		if e.Seq != wantSeqs[i] {
			t.Errorf("entries[%d].Seq = %d, want %d", i, e.Seq, wantSeqs[i])
		}
	}
	if next != 5 {
		t.Errorf("next = %d, want 5", next)
	}
}

func TestBuffer_SinceAfterOverflowReturnsRetainedTail(t *testing.T) {
	b := New(2)
	for i := range 10 {
		b.Add(LevelInfo, "entry %d", i)
	}

	entries, next := b.Since(0)
	if len(entries) != 2 {
		t.Fatalf("Since(0) returned %d entries, want 2", len(entries))
	}
	if entries[0].Seq != 8 || entries[1].Seq != 9 {
		t.Errorf("entries seqs = %d,%d, want 8,9", entries[0].Seq, entries[1].Seq)
	}
	if next != 10 {
		t.Errorf("next = %d, want 10", next)
	}

	// A cursor from before the eviction still gets whatever survived.
	entries, next = b.Since(3)
	if len(entries) != 2 || entries[0].Seq != 8 {
		t.Errorf("Since(3) = %+v, want the retained tail starting at seq 8", entries)
	}
	if next != 10 {
		t.Errorf("next = %d, want 10", next)
	}
}

func TestBuffer_SinceBeyondEndReturnsEmptyAndUnchangedNext(t *testing.T) {
	b := New(10)
	b.Add(LevelInfo, "only entry")

	entries, next := b.Since(50)
	if entries != nil {
		t.Errorf("entries = %+v, want nil/empty", entries)
	}
	if next != 1 {
		t.Errorf("next = %d, want 1", next)
	}

	// A repeat call with the returned cursor still yields nothing new.
	entries, next2 := b.Since(next)
	if entries != nil {
		t.Errorf("entries = %+v, want nil/empty", entries)
	}
	if next2 != next {
		t.Errorf("next2 = %d, want unchanged %d", next2, next)
	}
}

func TestBuffer_AddStripsControlCharacters(t *testing.T) {
	b := New(10)
	b.Add(LevelInfo, "bad\x00name\x01.flac\ttab")

	entries, _ := b.Since(0)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if want := "badname.flactab"; entries[0].Text != want {
		t.Errorf("Text = %q, want %q", entries[0].Text, want)
	}
}

func TestBuffer_NilReceiverIsSafe(t *testing.T) {
	var b *Buffer

	b.Add(LevelError, "should not panic")

	entries, next := b.Since(0)
	if entries != nil {
		t.Errorf("entries = %+v, want nil", entries)
	}
	if next != 0 {
		t.Errorf("next = %d, want 0", next)
	}
}

func TestBuffer_Concurrent(t *testing.T) {
	b := New(50)
	const goroutines = 8
	const perGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for i := range perGoroutine {
				b.Add(LevelInfo, "writer %d entry %d", id, i)
			}
		}(g)
		go func() {
			defer wg.Done()
			for range perGoroutine {
				b.Since(0)
			}
		}()
	}
	wg.Wait()

	entries, next := b.Since(0)
	if len(entries) != 50 {
		t.Fatalf("len(entries) = %d, want 50 (buffer capacity)", len(entries))
	}
	if next != goroutines*perGoroutine {
		t.Errorf("next = %d, want %d", next, goroutines*perGoroutine)
	}
}
