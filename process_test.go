package runner

import (
	"context"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNotStarted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.TODO(), 50*time.Second)
	defer cancel()
	p, err := NewProcess(ctx, "bash", "-c", "sleep 0.5 && echo hello && echo world 1>&2")
	require.NoError(t, err)

	// TODO: should be false, process not started
	//require.False(t, p.IsAlive())

	ctx1, cancel1 := context.WithTimeout(context.TODO(), 100*time.Millisecond)
	defer cancel1()
	err = p.StdOutScanner().WaitForKeyword(ctx1, "hello")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSearchOutputAfterExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.TODO(), 50*time.Second)
	defer cancel()
	p, err := NewProcess(ctx, "bash", "-c", "sleep 0.5 && echo hello && echo world 1>&2")
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
	p, err := NewProcess(ctx, "bash", "-c", "echo hello && sleep 5 && echo world && sleep 5")
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
