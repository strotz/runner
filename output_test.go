package runner

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeCloser struct {
	r io.Reader
}

func (f fakeCloser) Read(p []byte) (n int, err error) {
	return f.r.Read(p)
}

func (f fakeCloser) Close() error {
	return nil
}

func TestSearchSequence(t *testing.T) {
	source := `one
two
three
four
five`

	r := strings.NewReader(source)
	c := &fakeCloser{r: r}
	x := NewStreamScanner(c)
	require.NoError(t, x.WaitForKeyword(context.TODO(), "one"))
	require.NoError(t, x.WaitForKeyword(context.TODO(), "two"))
	require.NoError(t, x.WaitForKeyword(context.TODO(), "four"))
	require.NoError(t, x.WaitForKeyword(context.TODO(), "five"))
	require.Error(t, x.WaitForKeyword(context.TODO(), "three"))
}
