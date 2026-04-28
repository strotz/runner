package runner

import (
	"context"
	"io"
	"log"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNotStarted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.TODO(), 50*time.Second)
	defer cancel()
	p, err := NewProcess(ctx, nil, "bash", "-c", "sleep 0.5 && echo hello && echo world 1>&2")
	require.NoError(t, err)

	// TODO (Alexei): should be false, process not started
	// require.False(t, p.IsAlive())

	ctx1, cancel1 := context.WithTimeout(context.TODO(), 100*time.Millisecond)
	defer cancel1()
	err = p.StdOutScanner().WaitForKeyword(ctx1, "hello")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSearchOutputAfterExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.TODO(), 50*time.Second)
	defer cancel()
	p, err := NewProcess(ctx, nil, "bash", "-c", "sleep 0.5 && echo hello && echo world 1>&2")
	require.NoError(t, err)
	var wg sync.WaitGroup
	err = p.StartAsync(&wg)
	require.NoError(t, err)
	wg.Wait()
	log.Println("Process exits notification")

	require.False(t, p.IsAlive())
	r := p.NewStdOutReader()
	cout, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "hello\n", string(cout))

	err = p.StdOutScanner().WaitForKeyword(ctx, "hello")
	require.NoError(t, err)
	err = p.StdOutScanner().WaitForKeyword(ctx, "world")
	require.ErrorIs(t, err, KeywordNotFound)

	r = p.NewStdErrReader()
	cerr, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "world\n", string(cerr))
}

func TestSearchWhileWorking(t *testing.T) {
	ctx := context.TODO()
	// Long operation with 2 messages
	p, err := NewProcess(ctx, nil, "bash", "-c", "echo hello && sleep 5 && echo world && sleep 5")
	require.NoError(t, err)
	var wg sync.WaitGroup
	err = p.StartAsync(&wg)
	require.NoError(t, err)

	// Should be quick to find hello
	scanner := p.StdOutScanner()
	err = scanner.WaitForKeyword(ctx, "hello")
	require.NoError(t, err)

	// Should be timeout
	ctx1, cancel1 := context.WithTimeout(context.TODO(), 1*time.Second)
	defer cancel1()
	err = scanner.WaitForKeyword(ctx1, "world")
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// Should find now with longer context
	err = scanner.WaitForKeyword(ctx, "world")
	require.NoError(t, err)

	// Should exit with application
	err = scanner.WaitForKeyword(ctx, "blah")
	require.Error(t, err, KeywordNotFound)

	wg.Wait()
	log.Println("Process exits notification")
}

func TestSplit(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		lines := splitLines([]byte(""))
		require.Equal(t, [][]byte{{}}, lines)
	})

	t.Run("split with line separator", func(t *testing.T) {
		lines := splitLines([]byte("Hello\n"))
		require.Equal(t, [][]byte{[]byte("Hello")}, lines)
	})

	t.Run("split with line separator and empty line", func(t *testing.T) {
		lines := splitLines([]byte("Hello\n\n"))
		require.Equal(t, [][]byte{[]byte("Hello"), []byte("")}, lines)
	})

	t.Run("split with line separator and empty lines", func(t *testing.T) {
		lines := splitLines([]byte("Hello\n\n\n"))
		require.Equal(t, [][]byte{[]byte("Hello"), []byte(""), []byte("")}, lines)
	})
}

func TestWaitRepeatedly(t *testing.T) {
	var wg sync.WaitGroup

	// Long operation with 2 repeated messages
	p, err := NewProcess(t.Context(), nil, "bash", "-c", "echo hello && echo hello")
	require.NoError(t, err)
	require.NoError(t, p.StartAsync(&wg))

	require.NoError(t, p.StdOutScanner().WaitForKeyword(t.Context(), "hello"))
	require.NoError(t, p.StdOutScanner().WaitForKeyword(t.Context(), "hello"))
	require.ErrorIs(t, p.StdOutScanner().WaitForKeyword(t.Context(), "hello"), KeywordNotFound)
}

func TestProcessCancelContext(t *testing.T) {
	var wg sync.WaitGroup

	// Long operation with 2 repeated messages
	p, err := NewProcess(t.Context(), nil, "bash", "-c", "echo hello && echo hello && sleep 500000")
	require.NoError(t, err)
	require.NoError(t, p.StartAsync(&wg))

	// Read two keywords
	require.NoError(t, p.StdOutScanner().WaitForKeyword(t.Context(), "hello"))
	require.NoError(t, p.StdOutScanner().WaitForKeyword(t.Context(), "hello"))

	var errorFromGoroutine error

	synctest.Test(t, func(t *testing.T) {
		finished := false
		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			// let other goroutine start and wait for another hello
			errorFromGoroutine = p.StdOutScanner().WaitForKeyword(ctx, "hello")
			finished = true
		}()

		// Wait for the goroutine to start and block
		synctest.Wait()
		require.False(t, finished)

		// Cancel the process
		cancel()

		// Wait for the goroutine to finish
		synctest.Wait()
		require.True(t, finished)

		require.ErrorIs(t, errorFromGoroutine, context.Canceled)
	})
}
