package followparser

import (
	"bytes"
	"io"
	"log"
)

// scanBuffer holds the mutable state used while scanning a single file.
type scanBuffer struct {
	parser *Parser
	buf    []byte
	scan   int
	read   int64
	offset int
}

// newScanBuffer creates a scanBuffer initialized with the parser's StartBufSize.
func newScanBuffer(parser *Parser) *scanBuffer {
	return &scanBuffer{
		parser: parser,
		buf:    make([]byte, parser.StartBufSize),
	}
}

// parseLine parses a single line and updates counters.
func (s *scanBuffer) parseLine(line []byte) {
	if err := s.parser.Callback.Parse(line); err != nil {
		log.Printf("Failed to parse log :%v", err)
	}
	s.scan++
}

// flushTrailingLine handles a partial line remaining at the end of a file.
func (s *scanBuffer) flushTrailingLine(newest bool) {
	if s.offset == 0 || newest {
		return
	}
	s.read += int64(s.offset)
	s.parseLine(s.buf[0:s.offset])
}

// scanNewlines parses all complete lines in buf[0:n] and returns the index
// after the last newline that was consumed (or 0 if none were found).
func (s *scanBuffer) scanNewlines(n int) int {
	buf := s.buf[:n]
	k := 0
	for {
		idx := bytes.IndexByte(buf[k:], '\n')
		if idx < 0 {
			break
		}
		// found newline at k+idx
		lineEnd := k + idx
		s.read += int64(idx + 1)
		s.parseLine(buf[k:lineEnd])
		k = lineEnd + 1
	}
	return k
}

// compact moves any remaining partial line to the head of the buffer.
func (s *scanBuffer) compact(n, k int) {
	if k < n {
		copy(s.buf[0:], s.buf[k:n])
		s.offset = n - k
	} else {
		s.offset = 0
	}
}

// expand grows the buffer when it is full and contains no newlines.
func (s *scanBuffer) expand(n int) error {
	if n == s.parser.MaxBufSize {
		return ErrTokenTooLong
	}
	if n == len(s.buf) {
		newSize := len(s.buf) * 2
		newSize = min(newSize, s.parser.MaxBufSize)
		newBuf := make([]byte, newSize)
		copy(newBuf, s.buf)
		s.buf = newBuf
	}
	return nil
}

// readInto performs a single read into the buffer and returns the number of
// bytes read and whether EOF was reached. Non-EOF errors are returned as-is.
func (s *scanBuffer) readInto(f io.Reader) (int, bool, error) {
	nRead, err := f.Read(s.buf[s.offset:])
	if err == nil {
		return nRead, false, nil
	}
	if err == io.EOF {
		return nRead, true, nil
	}
	return nRead, false, err
}

// processChunk parses all complete lines from the current buffer contents and
// compacts any remaining partial line to the head of the buffer.
func (s *scanBuffer) processChunk(n int) {
	k := s.scanNewlines(n)
	s.compact(n, k)
}
