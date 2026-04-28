package runner

import (
	"context"
	"io"
)

// The sniffer is a tool that helps accumulate output of the process and provides search capability. It is
// useful for application tests.
type sniffer struct {
	out  io.Writer
	buf  *multiReaderBuffer
	main BufferSearch
}

func newSniffer(out io.Writer) *sniffer {
	buf := newMultiReaderBuffer()
	return &sniffer{
		out:  io.MultiWriter(out, buf),
		buf:  buf,
		main: buf.NewSearch(),
	}
}

func (s *sniffer) Write(p []byte) (int, error) {
	return s.out.Write(p)
}

func (s *sniffer) Close() error {
	return s.buf.Close()
}

func (s *sniffer) NewReader() io.ReadCloser {
	return s.buf.NewReader()
}

func (s *sniffer) NewSearch() BufferSearch {
	return s.buf.NewSearch()
}

// WaitForKeyword scans the output stream for given substr. It is a blocking call.
// It exits with nil when substr is found. It exits with KeywordNotFound it is not found, and the output stream is closed.
// When called repeatedly, it continues searching from the last found position.
func (s *sniffer) WaitForKeyword(ctx context.Context, substr string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return s.main.WaitForKeyword(ctx, substr)
}

// ScrollToEnd scrolls the main searcher to the end of the buffer.
func (s *sniffer) ScrollToEnd() {
	s.main.ScrollToEnd()
}
