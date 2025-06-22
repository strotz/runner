package runner

import (
	"io"
	"sync"
)

// MultiReaderBuffer is a thread safe memory buffer with one writer and multiple readers. I.e., it is
// possible to read the same buffer from start or continue reading while writing. Calling Close notifies all
// open and future readers that data is finalized and no more write operations are expected.
type MultiReaderBuffer interface {
	io.WriteCloser
	NewReader() io.ReadCloser
}

// multiReaderBuffer implements thread safe memory buffer with one writer and multiple readers.
type multiReaderBuffer struct {
	m      sync.Mutex
	cv     *sync.Cond
	buf    []byte
	closed bool
}

// NewMultiReaderBuffer returns new instance of MultiReaderBuffer
func NewMultiReaderBuffer() MultiReaderBuffer {
	r := &multiReaderBuffer{}
	r.cv = sync.NewCond(&r.m)
	return r
}

// Write writes data to the buffer and notifies all open readers that data is available.
func (b *multiReaderBuffer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	b.cv.L.Lock()
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	b.buf = append(b.buf, p...)
	b.cv.Broadcast()
	b.cv.L.Unlock()
	return len(p), nil
}

// NewReader returns new instance of Reader for the buffer.
func (b *multiReaderBuffer) NewReader() io.ReadCloser {
	return &multiBufferReader{source: b}
}

// Close closes the writer and notifies all open and future readers that data is finalized.
func (b *multiReaderBuffer) Close() error {
	b.cv.L.Lock()
	b.closed = true
	b.cv.Broadcast()
	b.cv.L.Unlock()
	return nil
}

type multiBufferReader struct {
	source *multiReaderBuffer
	offset int
	closed bool
}

// Read reads chunk from buffer. Blocks if there is no data to read, but the source is open.
// Read reads chunk from buffer. When reader is done with reading the
// content of the buffer, it will block until either more data arrives
// or buffer closed.
func (r *multiBufferReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n := 0
	sourceClosed := false
	r.source.cv.L.Lock()
	for {
		if r.closed {
			r.source.cv.L.Unlock()
			return 0, io.ErrClosedPipe
		}
		n = copy(p, r.source.buf[r.offset:])
		sourceClosed = r.source.closed
		if n == 0 && !sourceClosed {
			r.source.cv.Wait()
		} else {
			break
		}
	}
	r.source.cv.L.Unlock()
	if n == 0 {
		return 0, io.EOF
	}
	r.offset += n
	return n, nil
}

// Close closes the reader
func (r *multiBufferReader) Close() error {
	r.source.cv.L.Lock()
	r.closed = true
	r.source.cv.Broadcast()
	r.source.cv.L.Unlock()
	return nil
}
