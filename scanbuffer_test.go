package followparser

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// collectingCallback is a test Callback that records every parsed line.
type collectingCallback struct {
	lines []string
}

func (c *collectingCallback) Parse(b []byte) error {
	c.lines = append(c.lines, string(b))
	return nil
}

func (c *collectingCallback) Finish(float64) {}

// failingCallback returns a configured error for every parse.
type failingCallback struct {
	err error
}

func (f *failingCallback) Parse([]byte) error { return f.err }
func (f *failingCallback) Finish(float64)     {}

func newTestParser() (*Parser, *collectingCallback) {
	cb := &collectingCallback{}
	p := &Parser{
		Callback:     cb,
		StartBufSize: 16,
		MaxBufSize:   256,
	}
	return p, cb
}

func TestScanBufferProcessChunk(t *testing.T) {
	parser, cb := newTestParser()
	sb := newScanBuffer(parser)
	sb.buf = make([]byte, 32)

	copy(sb.buf, "line1\nline2\nline3")
	sb.processChunk(17)

	require.Equal(t, []string{"line1", "line2"}, cb.lines)
	require.Equal(t, 2, sb.scan)
	require.Equal(t, int64(12), sb.read)
	require.Equal(t, 5, sb.offset)
	require.Equal(t, "line3", string(sb.buf[0:sb.offset]))
}

func TestScanBufferFlushTrailingLine(t *testing.T) {
	parser, cb := newTestParser()
	sb := newScanBuffer(parser)
	sb.buf = make([]byte, 32)

	copy(sb.buf, "no-newline")
	sb.offset = 10
	sb.flushTrailingLine(false)

	require.Equal(t, []string{"no-newline"}, cb.lines)
	require.Equal(t, int64(10), sb.read)
}

func TestScanBufferFlushTrailingLineNewest(t *testing.T) {
	parser, cb := newTestParser()
	sb := newScanBuffer(parser)
	sb.buf = make([]byte, 32)

	copy(sb.buf, "no-newline")
	sb.offset = 10
	sb.flushTrailingLine(true)

	require.Empty(t, cb.lines)
	require.Equal(t, int64(0), sb.read)
}

func TestScanBufferReadInto(t *testing.T) {
	parser, _ := newTestParser()
	sb := newScanBuffer(parser)
	sb.buf = make([]byte, 32)

	r := strings.NewReader("hello")
	n, eof, err := sb.readInto(r)
	require.NoError(t, err)
	require.False(t, eof)
	require.Equal(t, 5, n)
	require.Equal(t, "hello", string(sb.buf[0:5]))
}

func TestScanBufferReadIntoWithPrefix(t *testing.T) {
	parser, _ := newTestParser()
	sb := newScanBuffer(parser)
	sb.buf = make([]byte, 32)

	copy(sb.buf, "pre-")
	sb.offset = 4

	r := strings.NewReader("fix")
	n, eof, err := sb.readInto(r)
	require.NoError(t, err)
	require.False(t, eof)
	require.Equal(t, 3, n)
	require.Equal(t, "pre-fix", string(sb.buf[0:7]))
}

func TestScanBufferReadIntoError(t *testing.T) {
	parser, _ := newTestParser()
	sb := newScanBuffer(parser)
	sb.buf = make([]byte, 32)

	errReader := &errorReader{err: io.ErrUnexpectedEOF}
	_, _, err := sb.readInto(errReader)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

type errorReader struct {
	err error
}

func (e *errorReader) Read([]byte) (int, error) {
	return 0, e.err
}

func TestScanBufferExpand(t *testing.T) {
	parser, _ := newTestParser()
	sb := newScanBuffer(parser)

	err := sb.expand(16)
	require.NoError(t, err)
	require.Equal(t, 32, len(sb.buf))
}

func TestScanBufferExpandMax(t *testing.T) {
	parser, _ := newTestParser()
	parser.MaxBufSize = 16
	sb := newScanBuffer(parser)

	err := sb.expand(16)
	require.ErrorIs(t, err, ErrTokenTooLong)
}

func TestScanBufferExpandCappedByMax(t *testing.T) {
	parser, _ := newTestParser()
	parser.StartBufSize = 16
	parser.MaxBufSize = 24
	sb := newScanBuffer(parser)

	err := sb.expand(16)
	require.NoError(t, err)
	require.Equal(t, 24, len(sb.buf))
}

func TestScanBufferCompactPartial(t *testing.T) {
	parser, _ := newTestParser()
	sb := newScanBuffer(parser)
	sb.buf = make([]byte, 32)

	copy(sb.buf, "abc123")
	sb.compact(6, 3)

	require.Equal(t, 3, sb.offset)
	require.Equal(t, "123", string(sb.buf[0:3]))
}

func TestScanBufferCompactAllConsumed(t *testing.T) {
	parser, _ := newTestParser()
	sb := newScanBuffer(parser)
	sb.buf = make([]byte, 32)

	copy(sb.buf, "abc")
	sb.offset = 3
	sb.compact(3, 3)

	require.Equal(t, 0, sb.offset)
}

func TestScanBufferScanNewlinesWithMultipleLines(t *testing.T) {
	parser, cb := newTestParser()
	sb := newScanBuffer(parser)
	sb.buf = make([]byte, 32)

	copy(sb.buf, "a\nb\nc")
	k := sb.scanNewlines(5)

	require.Equal(t, 4, k)
	require.Equal(t, []string{"a", "b"}, cb.lines)
	require.Equal(t, int64(4), sb.read)
}

func TestScanBufferParseLineError(t *testing.T) {
	cb := &failingCallback{err: io.ErrClosedPipe}
	parser := &Parser{
		Callback:     cb,
		StartBufSize: 16,
		MaxBufSize:   256,
	}
	sb := newScanBuffer(parser)

	// parseLine logs but does not return the error, so just verify it does not panic
	// and updates the scan counter.
	sb.parseLine([]byte("x"))
	require.Equal(t, 1, sb.scan)
}

func TestScanBufferScanFile(t *testing.T) {
	cb := &collectingCallback{}
	p := &Parser{
		Callback:     cb,
		StartBufSize: DefaultStartBufSize,
		MaxBufSize:   DefaultMaxBufSize,
		MaxReadSize:  DefaultMaxReadSize,
	}

	r := bytes.NewReader([]byte("one\ntwo\nthree"))
	rows, read, err := p.scanFile(r, false)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 3, rows)
	require.Equal(t, int64(13), read)
	require.Equal(t, []string{"one", "two", "three"}, cb.lines)
}

func TestScanBufferScanFileLongLine(t *testing.T) {
	cb := &collectingCallback{}
	p := &Parser{
		Callback:     cb,
		StartBufSize: 16,
		MaxBufSize:   256,
	}

	longLine := strings.Repeat("A", 100) + "\n"
	r := strings.NewReader(longLine)
	rows, read, err := p.scanFile(r, false)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 1, rows)
	require.Equal(t, int64(101), read)
	require.Equal(t, []string{strings.TrimSuffix(longLine, "\n")}, cb.lines)
}
