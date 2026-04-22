package logs

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestBuffer_WriteSplitsOnNewline(t *testing.T) {
	b := NewBuffer(10)
	if _, err := io.WriteString(b, "alpha\nbeta\ngamma\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := b.Tail(10)
	want := []string{"alpha", "beta", "gamma"}
	if !equalStrings(got, want) {
		t.Fatalf("Tail: got %v, want %v", got, want)
	}
}

func TestBuffer_TailEmpty(t *testing.T) {
	b := NewBuffer(5)
	if got := b.Tail(5); len(got) != 0 {
		t.Fatalf("empty Tail: got %v, want empty", got)
	}
	if got := b.Len(); got != 0 {
		t.Fatalf("empty Len: got %d, want 0", got)
	}
}

func TestBuffer_PartialLineBuffered(t *testing.T) {
	b := NewBuffer(5)
	_, _ = io.WriteString(b, "hel")
	_, _ = io.WriteString(b, "lo")
	if got := b.Tail(5); len(got) != 0 {
		t.Fatalf("unfinished line leaked: got %v", got)
	}
	_, _ = io.WriteString(b, " world\n")
	got := b.Tail(5)
	want := []string{"hello world"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuffer_OverflowDropsOldest(t *testing.T) {
	b := NewBuffer(3)
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(b, "line-%d\n", i)
	}
	got := b.Tail(10)
	want := []string{"line-3", "line-4", "line-5"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := b.Len(); got != 3 {
		t.Fatalf("Len: got %d, want 3", got)
	}
}

func TestBuffer_TailN(t *testing.T) {
	b := NewBuffer(100)
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(b, "l%d\n", i)
	}
	got := b.Tail(3)
	want := []string{"l8", "l9", "l10"}
	if !equalStrings(got, want) {
		t.Fatalf("Tail(3): got %v, want %v", got, want)
	}
	// Tail with n >= len returns all, oldest first.
	all := b.Tail(999)
	if len(all) != 10 || all[0] != "l1" || all[9] != "l10" {
		t.Fatalf("Tail(999): got %v", all)
	}
	// Tail(0) and negative return empty.
	if got := b.Tail(0); len(got) != 0 {
		t.Fatalf("Tail(0): got %v", got)
	}
	if got := b.Tail(-1); len(got) != 0 {
		t.Fatalf("Tail(-1): got %v", got)
	}
}

func TestBuffer_StripsCarriageReturn(t *testing.T) {
	b := NewBuffer(5)
	_, _ = io.WriteString(b, "one\r\ntwo\r\n")
	got := b.Tail(5)
	want := []string{"one", "two"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuffer_HugePartialFlushes(t *testing.T) {
	b := NewBuffer(2)
	// Write more than maxLineBytes without a newline; buffer must not grow
	// unbounded and must eventually emit a line.
	huge := strings.Repeat("x", maxLineBytes+100)
	_, _ = io.WriteString(b, huge)
	if got := b.Len(); got == 0 {
		t.Fatalf("expected a forced flush after %d bytes, got 0 lines", maxLineBytes)
	}
}

func TestBuffer_ReturnedTailIsACopy(t *testing.T) {
	b := NewBuffer(5)
	_, _ = io.WriteString(b, "a\nb\n")
	got := b.Tail(5)
	got[0] = "mutated"
	fresh := b.Tail(5)
	if fresh[0] != "a" {
		t.Fatalf("Tail must return a copy; internal state mutated: %v", fresh)
	}
}

func TestBuffer_ZeroCapacityClampsToOne(t *testing.T) {
	b := NewBuffer(0)
	_, _ = io.WriteString(b, "first\nsecond\n")
	got := b.Tail(5)
	want := []string{"second"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuffer_ConcurrentWrites(t *testing.T) {
	b := NewBuffer(1000)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				fmt.Fprintf(b, "writer-%d-line-%d\n", id, j)
			}
		}(i)
	}
	wg.Wait()
	if got := b.Len(); got != 500 {
		t.Fatalf("Len after concurrent writes: got %d, want 500", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
