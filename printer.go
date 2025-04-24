package runner

import (
	"bytes"
	"fmt"
	"io"
)

// FormattedPrinter is a custom io.Writer that formats the output with a prefix.
type FormattedPrinter struct {
	Out    io.Writer
	Prefix string
}

func (f *FormattedPrinter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// Split to multiple lines
	lines := bytes.Split(p, []byte("\n"))
	for _, line := range lines {
		_, err := fmt.Fprintf(f.Out, "%-16.16s| %s\n", f.Prefix, line)
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
