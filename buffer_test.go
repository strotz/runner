package runner

import (
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteOnceReadMany(t *testing.T) {
	buf := NewMultiReaderBuffer()
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
	buf := NewMultiReaderBuffer()
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
	buf := NewMultiReaderBuffer()
	n, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	reader := buf.NewReader()
	v1 := make([]byte, 10)
	read, err := reader.Read(v1)
	require.NoError(t, err)
	require.Equal(t, 5, read)
	require.Equal(t, "hello", string(v1[0:read]))

	var wg sync.WaitGroup
	started := make(chan struct{})
	wg.Add(1)
	go func() {
		close(started)
		read, err = reader.Read(v1)
		assert.NoError(t, err)
		assert.Equal(t, 5, read)
		wg.Done()
	}()

	// wait for goroutine to start
	<-started

	// reader is blocked, send data to unblock
	n, err = buf.Write([]byte("world"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	// Wait for the reader to finish
	wg.Wait()
}

func TestReadBlocksUntilSourceClosed(t *testing.T) {
	buf := NewMultiReaderBuffer()
	n, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	reader := buf.NewReader()
	v1 := make([]byte, 10)
	read, err := reader.Read(v1)
	require.NoError(t, err)
	require.Equal(t, 5, read)
	require.Equal(t, "hello", string(v1[0:read]))

	var wg sync.WaitGroup
	started := make(chan struct{})
	wg.Add(1)
	go func() {
		close(started)
		read, err = reader.Read(v1)
		assert.Equal(t, err, io.EOF)
		wg.Done()
	}()

	// wait for goroutine to start
	<-started

	// reader is blocked, close to unblock
	err = buf.Close()
	require.NoError(t, err)

	// wait for the reader to finish
	wg.Wait()
}

func TestReadBlocksUntilReaderClosed(t *testing.T) {
	buf := NewMultiReaderBuffer()
	n, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	reader := buf.NewReader()
	v1 := make([]byte, 10)
	read, err := reader.Read(v1)
	require.NoError(t, err)
	require.Equal(t, 5, read)
	require.Equal(t, "hello", string(v1[0:read]))

	var wg sync.WaitGroup
	started := make(chan struct{})
	wg.Add(1)
	go func() {
		close(started)
		_, err = reader.Read(v1)
		assert.EqualError(t, err, "io: read/write on closed pipe")
		wg.Done()
	}()

	// wait for goroutine to start
	<-started

	// reader is blocked, close to unblock
	err = reader.Close()
	require.NoError(t, err)

	// wait for the reader to finish
	wg.Wait()
}
