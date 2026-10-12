package streamwatch

import "sync"

// Reuse only the small initial scanner storage. Expanded multi-megabyte lines
// belong to the scanner and must not be retained in the process-wide pool.
var lineBuffers = sync.Pool{New: func() any { return new([64 << 10]byte) }}

func BorrowLineBuffer() ([]byte, func()) {
	buffer := lineBuffers.Get().(*[64 << 10]byte)
	return buffer[:], func() { lineBuffers.Put(buffer) }
}
