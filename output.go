package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

var KeywordNotFound = errors.New("failed to find keyword")

// The AccumulatedOutput is a tool that helps accumulate output of the process and provides search capability. It is
// useful for application tests.
type AccumulatedOutput struct {
	out io.Writer
	buf MultiReaderBuffer
}

func NewAccumulatedOutput(out io.Writer) *AccumulatedOutput {
	buf := NewMultiReaderBuffer()
	return &AccumulatedOutput{
		out: io.MultiWriter(out, buf),
		buf: buf,
	}
}

func (s *AccumulatedOutput) Write(p []byte) (int, error) {
	return s.out.Write(p)
}

func (s *AccumulatedOutput) Close() error {
	return s.buf.Close()
}

func (s *AccumulatedOutput) NewReader() io.ReadCloser {
	return s.buf.NewReader()
}

// WaitForKeyword scans the output stream for given substr. It is a blocking call.
// It exits with nil when substr is found. It exits with KeywordNotFound it is not found, and the output stream is closed.
func (s *AccumulatedOutput) WaitForKeyword(ctx context.Context, substr string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	cr := &cancellableReader{
		reader: s.NewReader(),
		ctx:    ctx,
	}
	scanner := bufio.NewScanner(cr)
	return waitForKeyword(ctx, scanner, substr)
}

type cancellableReader struct {
	reader io.ReadCloser
	ctx    context.Context
}

type readerResults struct {
	n   int
	err error
}

func (r *cancellableReader) Read(p []byte) (int, error) {
	retCh := make(chan readerResults)

	go func() {
		n, err := r.reader.Read(p)
		retCh <- readerResults{n, err}
	}()

	// Waits for either result from reader or for the context cancellation
	select {
	case res := <-retCh:
		return res.n, res.err
	case <-r.ctx.Done():
		// Notify the reader that we do not care anymore.
		_ = r.reader.Close()
		return 0, r.ctx.Err()
	}
}

type StreamScanner struct {
	source  io.ReadCloser
	reader  *cancellableReader
	scanner *bufio.Scanner
}

func NewStreamScanner(r io.ReadCloser) *StreamScanner {
	return &StreamScanner{
		source: r,
	}
}

func (s *StreamScanner) WaitForKeyword(ctx context.Context, substr string) error {
	// Should we iniialize reader and scanner?
	if s.reader == nil {
		s.reader = &cancellableReader{
			reader: s.source,
			ctx:    ctx,
		}
		s.scanner = bufio.NewScanner(s.reader)
	}
	return waitForKeyword(ctx, s.scanner, substr)
}

func waitForKeyword(ctx context.Context, scanner *bufio.Scanner, substr string) error {
	for scanner.Scan() {
		line := scanner.Text()
		found := strings.Contains(line, substr)
		if found {
			slog.Debug("Message", substr, "found")
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("%w '%s' in output", KeywordNotFound, substr)
}
