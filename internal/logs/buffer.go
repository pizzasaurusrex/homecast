package logs

import "sync"

// maxLineBytes caps the size of a single in-flight partial line. If a
// writer produces this many bytes without a newline, we force-flush the
// accumulated bytes as one line to bound memory.
const maxLineBytes = 64 * 1024

// Buffer is a bounded ring buffer of recent log lines. It is safe for concurrent use by multiple writers and readers.
type Buffer struct {
	mu       sync.Mutex
	lines    []string
	head     int // index where the next line will be written
	count    int // number of retained lines (<= cap(lines))
	partial  []byte
	capacity int
}

// NewBuffer returns a Buffer that retains up to capacity lines. A
// non-positive capacity is clamped to 1 so the Writer is always usable.
func NewBuffer(capacity int) *Buffer {
	if capacity < 1 {
		capacity = 1
	}
	return &Buffer{
		lines:    make([]string, capacity),
		capacity: capacity,
	}
}

// Write implements io.Writer. It never errors and always reports len(p)
// consumed — log producers should not be blocked by the buffer.
func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := len(p)
	start := 0
	for i, c := range p {
		if c == '\n' {
			b.appendLineLocked(append(b.partial, p[start:i]...))
			b.partial = b.partial[:0]
			start = i + 1
		}
	}
	if start < len(p) {
		b.partial = append(b.partial, p[start:]...)
	}
	if len(b.partial) >= maxLineBytes {
		b.appendLineLocked(b.partial)
		b.partial = b.partial[:0]
	}
	return n, nil
}

// Tail returns up to n most recent lines, oldest first. A non-positive n
// returns an empty slice. The returned slice is a fresh copy.
func (b *Buffer) Tail(n int) []string {
	if n <= 0 {
		return []string{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if n > b.count {
		n = b.count
	}
	out := make([]string, n)
	start := (b.head - n + b.capacity) % b.capacity
	for i := 0; i < n; i++ {
		out[i] = b.lines[(start+i)%b.capacity]
	}
	return out
}

// Len returns the number of retained lines.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count
}

func (b *Buffer) appendLineLocked(line []byte) {
	// Strip a single trailing '\r' so CRLF-terminated writers produce clean lines.
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	b.lines[b.head] = string(line)
	b.head = (b.head + 1) % b.capacity
	if b.count < b.capacity {
		b.count++
	}
}
