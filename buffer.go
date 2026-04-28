package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

// KeywordNotFound is returned when a searched keyword is not found in the buffer and the source is closed.
var KeywordNotFound = errors.New("failed to find keyword")

// BufferSearch defines interface for searching keyword in a stream of data.
type BufferSearch interface {
	// WaitForKeyword scans the output stream for given substr. It is a blocking call. It exits with nil when substr is found.
	// It exits with KeywordNotFound it is not found, and the output stream is closed.
	WaitForKeyword(ctx context.Context, substr string) error
	// ScrollToEnd moves the search offset to the end of the buffer.
	ScrollToEnd()
}

// multiReaderBuffer is a thread safe memory buffer with one writer and multiple readers. I.e., it is
// possible to read the same buffer from start or continue reading while writing. Calling Close notifies all
// open and future readers that data is finalized and no more write operations are expected.
type multiReaderBuffer struct {
	m      sync.Mutex
	cv     *sync.Cond
	buf    []byte
	closed bool
}

// newMultiReaderBuffer returns new instance of MultiReaderBuffer
func newMultiReaderBuffer() *multiReaderBuffer {
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
	defer b.cv.L.Unlock()
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	b.buf = append(b.buf, p...)
	b.cv.Broadcast()
	return len(p), nil
}

// NewReader returns new instance of Reader for the buffer.
func (b *multiReaderBuffer) NewReader() io.ReadCloser {
	return &multiBufferReader{source: b}
}

// Close closes the writer and notifies all open and future readers that data is finalized.
func (b *multiReaderBuffer) Close() error {
	b.cv.L.Lock()
	defer b.cv.L.Unlock()
	b.closed = true
	b.cv.Broadcast()
	return nil
}

// waitOrCancel waits until either the condition variable is signaled or the context is canceled.
// If the context is canceled, it notifies all waiting readers.
func (b *multiReaderBuffer) waitOrCancel(ctx context.Context) error {
	// Do not need to start if context is already canceled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Thankfully, someone invented context.AfterFunc https://pkg.go.dev/context#example-AfterFunc-Cond
	stopf := context.AfterFunc(ctx, func() {
		b.cv.L.Lock()
		defer b.cv.L.Unlock()
		b.cv.Broadcast()
	})
	defer stopf()

	// wait until either condition variable is signaled or context is canceled
	b.cv.Wait()

	return ctx.Err()
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

type bufferSearcher struct {
	source *multiReaderBuffer
	offset int
}

// NewSearch returns the instance of BufferSearch for the buffer.
func (b *multiReaderBuffer) NewSearch() BufferSearch {
	return &bufferSearcher{
		source: b,
		offset: 0,
	}
}

// WaitForKeyword scans the output stream for given substr. It is a blocking call.
// It exits with nil when substr is found. It exits with KeywordNotFound it is not found, and the output stream is closed.
// When called repeatedly, it continues searching from the last found position.
func (b *bufferSearcher) WaitForKeyword(ctx context.Context, substr string) error {
	// Do not need to start if context is already canceled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	substrBytes := []byte(substr)
	substrLen := len(substrBytes)
	if substrLen == 0 {
		return nil
	}

	b.source.cv.L.Lock()
	defer b.source.cv.L.Unlock()
	for {
		// search for substring in the available buffer
		tail := b.source.buf[b.offset:]
		n := bytes.Index(tail, substrBytes)
		if n >= 0 {
			// found, move offset to the end of found substring and return
			b.offset += n + substrLen
			return nil
		}
		// not found
		// if source is closed, return KeywordNotFound
		if b.source.closed {
			return fmt.Errorf("%w: \"%s\"", KeywordNotFound, substr)
		}
		// if the source is open, wait until more data is available or context is canceled
		if len(tail) >= substrLen {
			// Optimization here: move offset to the position where a match is still possible
			b.offset += len(tail) - substrLen + 1
		}
		// wait until more data is available or context is canceled
		if err := b.source.waitOrCancel(ctx); err != nil {
			return err
		}
	}
}

func (b *bufferSearcher) currentOffset() int {
	b.source.cv.L.Lock()
	defer b.source.cv.L.Unlock()
	return b.offset
}

// ScrollToEnd moves the search offset to the end of the buffer.
func (b *bufferSearcher) ScrollToEnd() {
	b.source.cv.L.Lock()
	defer b.source.cv.L.Unlock()
	b.offset = len(b.source.buf)
}
