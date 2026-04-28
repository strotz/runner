package runner

import (
	"bytes"
	"fmt"
	"io"
)

type formattedPrinter struct {
	out  io.Writer
	name string
}

// splitLines splits the input byte slice into lines using '\n' as a delimiter.
func splitLines(p []byte) [][]byte {
	newline := []byte("\n")
	return bytes.Split(bytes.TrimSuffix(p, newline), newline)
}

func (f *formattedPrinter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// Split to multiple lines
	lines := splitLines(p)
	for _, line := range lines {
		_, err := fmt.Fprintf(f.out, "%-16.16s| %s\n", f.name, line)
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
