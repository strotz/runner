package runner

import (
	"context"
	"io"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteOnceReadMany(t *testing.T) {
	buf := newMultiReaderBuffer()
	n, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	err = buf.Close()
	require.NoError(t, err)

	reader1 := buf.NewReader()
	v1 := make([]byte, 10)
	read, err := reader1.Read(v1)
	require.NoError(t, err)
	require.Equal(t, 5, read)
	require.Equal(t, "hello", string(v1[0:read]))

	reader2 := buf.NewReader()
	v2 := make([]byte, 10)
	read, err = reader2.Read(v2)
	require.NoError(t, err)
	require.Equal(t, 5, read)
	require.Equal(t, "hello", string(v2[0:read]))
}

func TestWriteTwiceReadMany(t *testing.T) {
	buf := newMultiReaderBuffer()
	n, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	reader1 := buf.NewReader()
	v1 := make([]byte, 10)
	read, err := reader1.Read(v1)
	require.NoError(t, err)
	require.Equal(t, 5, read)
	require.Equal(t, "hello", string(v1[0:read]))

	n, err = buf.Write([]byte("world"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	reader2 := buf.NewReader()
	v2 := make([]byte, 20)
	read, err = reader2.Read(v2)
	require.NoError(t, err)
	require.Equal(t, 10, read)
	require.Equal(t, "helloworld", string(v2[0:read]))

	read, err = reader1.Read(v1)
	require.NoError(t, err)
	require.Equal(t, 5, read)
	require.Equal(t, "world", string(v1[0:read]))
}

func TestReadBlocksUntilMoreData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := newMultiReaderBuffer()
		n, err := buf.Write([]byte("hello"))
		require.NoError(t, err)
		require.Equal(t, 5, n)

		reader := buf.NewReader()
		v1 := make([]byte, 10)
		read, err := reader.Read(v1)
		require.NoError(t, err)
		require.Equal(t, 5, read)
		require.Equal(t, "hello", string(v1[0:read]))

		finished := false
		go func() {
			read, err := reader.Read(v1)
			finished = true
			assert.NoError(t, err)
			assert.Equal(t, 5, read)
		}()

		// reader is blocked, wait for it
		synctest.Wait()
		require.False(t, finished)

		// reader is blocked, send data to unblock
		n, err = buf.Write([]byte("world"))
		require.NoError(t, err)
		require.Equal(t, 5, n)

		// Wait for go routine to finish
		synctest.Wait()
		require.True(t, finished)
	})
}

func TestReadBlocksUntilSourceClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := newMultiReaderBuffer()
		n, err := buf.Write([]byte("hello"))
		require.NoError(t, err)
		require.Equal(t, 5, n)

		reader := buf.NewReader()
		v1 := make([]byte, 10)
		read, err := reader.Read(v1)
		require.NoError(t, err)
		require.Equal(t, 5, read)
		require.Equal(t, "hello", string(v1[0:read]))

		finished := false
		go func() {
			_, err := reader.Read(v1)
			finished = true
			assert.Equal(t, err, io.EOF)
		}()

		// reader is blocked, wait for it
		synctest.Wait()
		require.False(t, finished)

		// reader is blocked, close to unblock
		err = buf.Close()
		require.NoError(t, err)

		// wait for the goroutine to finish
		synctest.Wait()
		require.True(t, finished)
	})
}

// TestReadBlocksUntilReaderClosed validates that read returns io.ErrClosedPipe when the reader is
// closed while it is blocked on reading.
func TestReadBlocksUntilReaderClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := newMultiReaderBuffer()
		n, err := buf.Write([]byte("hello"))
		require.NoError(t, err)
		require.Equal(t, 5, n)

		reader := buf.NewReader()
		v1 := make([]byte, 10)
		read, err := reader.Read(v1)
		require.NoError(t, err)
		require.Equal(t, 5, read)
		require.Equal(t, "hello", string(v1[0:read]))

		finished := false
		go func() {
			_, err := reader.Read(v1)
			finished = true
			assert.EqualError(t, err, "io: read/write on closed pipe")
		}()

		synctest.Wait()
		require.False(t, finished)

		// reader is blocked, close to unblock
		err = reader.Close()
		require.NoError(t, err)

		// wait for the reader to finish
		synctest.Wait()
		require.True(t, finished)
	})
}

func TestSearch(t *testing.T) {
	buf := newMultiReaderBuffer()
	n, err := buf.Write([]byte("hello\nworld\none\ntwo\nthree\nfour\nfive\n"))
	require.NoError(t, err)
	require.Equal(t, 36, n)

	search := buf.NewSearch()
	err = search.WaitForKeyword(context.TODO(), "hello")
	require.NoError(t, err)
	err = search.WaitForKeyword(context.TODO(), "world")
	require.NoError(t, err)
	err = search.WaitForKeyword(context.TODO(), "four")
	require.NoError(t, err)
	err = search.WaitForKeyword(context.TODO(), "five")
	require.NoError(t, err)

	err = buf.Close()
	require.NoError(t, err)

	// Try to search for something that already passed
	err = search.WaitForKeyword(context.TODO(), "world")
	require.ErrorIs(t, err, KeywordNotFound)
}

func TestSearchBlocksUntilMoreData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := newMultiReaderBuffer()
		n, err := buf.Write([]byte("hello\nworld\none\ntwo\nthree\nfour\nfive\n"))
		require.NoError(t, err)
		require.Equal(t, 36, n)

		search := buf.NewSearch()
		require.NoError(t, search.WaitForKeyword(context.TODO(), "hello"))
		require.NoError(t, search.WaitForKeyword(context.TODO(), "world"))

		finished := false
		go func() {
			err := search.WaitForKeyword(context.TODO(), "six")
			finished = true
			assert.NoError(t, err)
		}()

		// search is blocked, wait for it
		synctest.Wait()
		require.False(t, finished)

		// search is blocked, send data to unblock
		n, err = buf.Write([]byte("six\n"))
		require.NoError(t, err)
		require.Equal(t, 4, n)

		// wait for go routine to finish
		synctest.Wait()
		require.True(t, finished)
	})
}

// TestSearchBlocksUntilSourceClosed validates that the search returns KeywordNotFound when the
// source is closed while it is waiting for input.
func TestSearchBlocksUntilSourceClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := newMultiReaderBuffer()
		n, err := buf.Write([]byte("hello\nworld\none\ntwo\nthree\nfour\nfive\n"))
		require.NoError(t, err)
		require.Equal(t, 36, n)

		search := buf.NewSearch()
		require.NoError(t, search.WaitForKeyword(context.TODO(), "hello"))
		require.NoError(t, search.WaitForKeyword(context.TODO(), "world"))

		finished := false
		go func() {
			err := search.WaitForKeyword(context.TODO(), "six")
			finished = true
			assert.ErrorIs(t, err, KeywordNotFound)
		}()

		// search is blocked, wait for it
		synctest.Wait()
		require.False(t, finished)

		// search is blocked, close to unblock
		require.NoError(t, buf.Close())

		// wait for the go routine to finish
		synctest.Wait()
		require.True(t, finished)
	})
}

// TestSearchWithContextCancel validates that the search returns context.Canceled when the context
// is canceled while it is waiting for input.
func TestSearchWithContextCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := newMultiReaderBuffer()
		n, err := buf.Write([]byte("hello\nworld\none\ntwo\nthree\nfour\nfive\n"))
		require.NoError(t, err)
		require.Equal(t, 36, n)

		search := buf.NewSearch()
		require.NoError(t, search.WaitForKeyword(context.TODO(), "hello"))
		require.NoError(t, search.WaitForKeyword(context.TODO(), "world"))

		ctx, cancel := context.WithCancel(context.TODO())

		finished := false
		go func() {
			err := search.WaitForKeyword(ctx, "six")
			finished = true
			assert.ErrorIs(t, err, context.Canceled)
		}()

		// search is blocked, wait for it
		synctest.Wait()
		require.False(t, finished)

		// cancel the context to unblock the search
		cancel()

		// wait for the search to finish
		synctest.Wait()
		require.True(t, finished)
	})
}

// TestSearchContextAndUpdateOffset validates that the search returns KeywordNotFound when the
// source is closed while it is waiting for input at proper offset of the buffer.
func TestSearchContextAndUpdateOffset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := newMultiReaderBuffer()
		n, err := buf.Write([]byte("hello\nworld\none\ntwo\nthree\nfour\nfive\n"))
		require.NoError(t, err)
		require.Equal(t, 36, n)

		search := buf.NewSearch()

		finished := false
		go func() {
			err := search.WaitForKeyword(context.TODO(), "six")
			finished = true
			assert.ErrorIs(t, err, KeywordNotFound)
		}()

		// search is blocked, wait for it
		synctest.Wait()
		require.False(t, finished)

		offset := search.(*bufferSearcher).currentOffset()
		require.Equal(t, 34, offset) // after "five\n"

		require.NoError(t, buf.Close())

		// wait for the search to finish
		synctest.Wait()
		require.True(t, finished)
	})
}
