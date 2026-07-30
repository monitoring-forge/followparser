package followparser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type testParser struct {
	buf      *bytes.Buffer
	duration float64
}

// Helper to write a test file with repeated lines
func writeTestFile(path string, line string, count int) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	for range count {
		if _, err := fh.WriteString(line); err != nil {
			return err
		}
	}
	return fh.Sync()
}

func benchScannerFile(b *testing.B, fname string) {
	b.ReportAllocs()
	for range b.N {
		fh, err := os.Open(fname)
		if err != nil {
			b.Fatal(err)
		}
		parser := &dummyParser{}
		scanner := bufio.NewScanner(fh)
		scanner.Buffer(make([]byte, DefaultStartBufSize), DefaultMaxBufSize)
		for scanner.Scan() {
			if err := parser.Parse(scanner.Bytes()); err != nil {
				b.Fatal(err)
			}
		}
		if err := scanner.Err(); err != nil {
			b.Fatal(err)
		}
		fh.Close()
	}
}

func benchScanFile(b *testing.B, fname string) {
	b.ReportAllocs()
	for range b.N {
		fh, err := os.Open(fname)
		if err != nil {
			b.Fatal(err)
		}
		parser := &dummyParser{}
		p := &Parser{
			Callback:     parser,
			StartBufSize: DefaultStartBufSize,
			MaxBufSize:   DefaultMaxBufSize,
			MaxReadSize:  DefaultMaxReadSize,
		}
		_, _, err = p.scanFile(fh, true)
		if err != nil && !errors.Is(err, io.EOF) {
			b.Fatal(err)
		}
		fh.Close()
	}
}

func BenchmarkScanner_SmallLines(b *testing.B) {
	dir := b.TempDir()
	fname := filepath.Join(dir, "small.log")
	line := "short line example\n"
	// ~10k lines
	if err := writeTestFile(fname, line, 10000); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	benchScannerFile(b, fname)
}

func BenchmarkScanFile_SmallLines(b *testing.B) {
	dir := b.TempDir()
	fname := filepath.Join(dir, "small.log")
	line := "short line example\n"
	if err := writeTestFile(fname, line, 10000); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	benchScanFile(b, fname)
}

func BenchmarkScanner_LongLine(b *testing.B) {
	dir := b.TempDir()
	fname := filepath.Join(dir, "long.log")
	longLine := string(bytes.Repeat([]byte("A"), DefaultStartBufSize+100)) + "\n"
	// single long line
	if err := writeTestFile(fname, longLine, 1); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	benchScannerFile(b, fname)
}

func BenchmarkScanFile_LongLine(b *testing.B) {
	dir := b.TempDir()
	fname := filepath.Join(dir, "long.log")
	longLine := string(bytes.Repeat([]byte("A"), DefaultStartBufSize+100)) + "\n"
	if err := writeTestFile(fname, longLine, 1); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	benchScanFile(b, fname)
}

func (p *testParser) Parse(b []byte) error {
	p.buf.Write(b)
	p.buf.WriteString("\n")
	return nil
}

func (p *testParser) Finish(duration float64) {
	p.duration = duration
}

func (p *testParser) Slurp() *bytes.Buffer {
	return p.buf
}

func TestParse(t *testing.T) {
	tmpdir := t.TempDir()
	logFileName := filepath.Join(tmpdir, "log")
	fh, err := os.Create(logFileName)
	require.NoError(t, err, "failed to create log file")

	for i := range 2 {
		buf := bytes.NewBufferString("")
		parser := &testParser{
			buf:      buf,
			duration: 0,
		}
		msg := fmt.Sprintf("msg msg %08d\n", i)
		_, _ = fh.WriteString(msg)
		fp := &Parser{
			WorkDir:  tmpdir,
			Callback: parser,
		}
		r, err := fp.Parse("logPos", logFileName)
		require.NoError(t, err, "failed to parse log file")

		out := parser.Slurp().String()
		require.Equal(t, msg, out, "read output does not match expected")
		require.Len(t, r, 1, "result len must be 1")
		require.Equal(t, 1, r[0].Rows, "result[0].Rows must be 1")
		require.Equal(t, int64(17), r[0].EndPos-r[0].StartPos, "r[0].EndPos - r[0].StartPos must be 17")
	}

	time.Sleep(time.Second)
	msg3 := fmt.Sprintf("msg msg %08d\n", 3)
	_, _ = fh.WriteString(msg3)
	fh.Close()
	_ = os.Rename(logFileName, filepath.Join(tmpdir, "log.1"))
	fh, err = os.Create(logFileName)
	require.NoError(t, err, "failed to create new log file")

	msg4 := fmt.Sprintf("msg msg %08d\n", 4)
	_, _ = fh.WriteString(msg4)
	buf := bytes.NewBufferString("")
	parser := &testParser{
		buf:      buf,
		duration: 0,
	}
	fp := &Parser{
		WorkDir:  tmpdir,
		Callback: parser,
		Silent:   true,
	}
	r, err := fp.Parse("logPos", logFileName)
	require.NoError(t, err, "failed to parse log file")

	out := parser.Slurp().String()
	require.Equal(t, msg3+msg4, out, "read output does not match expected")
	require.GreaterOrEqual(t, parser.duration, 1.0, "duration must be at least 1")
	require.Len(t, r, 2, "result len must be 2")
	require.Equal(t, 1, r[0].Rows, "result[0].Rows must be 1")
	require.Equal(t, 1, r[1].Rows, "result[1].Rows must be 1")

	// --- Archive directory move test starts here ---
	archiveDir := filepath.Join(tmpdir, "archive")
	err = os.Mkdir(archiveDir, 0755)
	require.NoError(t, err, "failed to create archive directory")

	// Append to the log file
	fh.Close()
	fh, err = os.OpenFile(logFileName, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err, "failed to open log file for appending")

	msg5 := fmt.Sprintf("msg msg %08d\n", 5)
	_, _ = fh.WriteString(msg5)
	fh.Close()
	// Move log file to archive directory
	archivedLog := filepath.Join(archiveDir, "log.2")
	err = os.Rename(logFileName, archivedLog)
	require.NoError(t, err, "failed to move log file to archive directory")

	// Create a new log file and write to it
	fh, err = os.Create(logFileName)
	require.NoError(t, err, "failed to create new log file")

	msg6 := fmt.Sprintf("msg msg %08d\n", 6)
	_, _ = fh.WriteString(msg6)
	fh.Close()
	buf = bytes.NewBufferString("")
	parser = &testParser{
		buf:      buf,
		duration: 0,
	}
	fp = &Parser{
		WorkDir:    tmpdir,
		Callback:   parser,
		ArchiveDir: archiveDir,
		Silent:     true,
	}
	r, err = fp.Parse("logPos", logFileName)
	require.NoError(t, err, "failed to parse log file after archive move")

	out = parser.Slurp().String()
	require.Equal(t, msg5+msg6, out, "archive follow read output does not match expected")
	require.Len(t, r, 2, "archive follow result len must be 2")
	require.Equal(t, 1, r[0].Rows, "archive follow result[0].Rows must be 1")
	require.Equal(t, 1, r[1].Rows, "archive follow result[1].Rows must be 1")
}

func TestParseWithNoCommitPosFile(t *testing.T) {
	tmpdir := t.TempDir()
	logFileName := filepath.Join(tmpdir, "log")
	fh, err := os.Create(logFileName)
	require.NoError(t, err, "failed to create log file")

	lastmsg := strings.Builder{}
	var fp *Parser
	for i := range 2 {
		buf := bytes.NewBufferString("")
		parser := &testParser{
			buf:      buf,
			duration: 0,
		}
		msg := fmt.Sprintf("msg msg %08d\n", i)
		lastmsg.WriteString(msg)
		_, _ = fh.WriteString(msg)
		fp = &Parser{
			WorkDir:             tmpdir,
			Callback:            parser,
			NoAutoCommitPosFile: true,
		}
		r, err := fp.Parse("logPos", logFileName)
		require.NoError(t, err, "failed to parse log file with NoAutoCommitPosFile")

		out := parser.Slurp().String()
		require.Equal(t, lastmsg.String(), out, "read output does not match expected")
		require.Len(t, r, 1, "result len must be 1")
		require.Equal(t, i+1, r[0].Rows, "result[0].Rows must be i+1")
		require.Equal(t, int64(17*(i+1)), r[0].EndPos-r[0].StartPos, "r[0].EndPos - r[0].StartPos must be 17*(i+1)")
	}
	errCommit := fp.CommitPosFile()
	require.NoError(t, errCommit, "failed to commit pos file")

	{
		buf := bytes.NewBufferString("")
		parser := &testParser{
			buf:      buf,
			duration: 0,
		}
		msg3 := fmt.Sprintf("msg msg %08d\n", 3)
		_, _ = fh.WriteString(msg3)
		fp = &Parser{
			WorkDir:             tmpdir,
			Callback:            parser,
			NoAutoCommitPosFile: false,
		}
		r, err := fp.Parse("logPos", logFileName)
		require.NoError(t, err, "failed to parse log file after committing pos file")

		out := parser.Slurp().String()
		require.Equal(t, msg3, out, "read output does not match expected")
		require.Len(t, r, 1, "result len must be 1")
		require.Equal(t, 1, r[0].Rows, "result[0].Rows must be 1")
		require.Equal(t, int64(17), r[0].EndPos-r[0].StartPos, "r[0].EndPos - r[0].StartPos must be 17")
	}
}

// Test when the last line in the log file does not end with a newline,
// then the file is appended to. Ensure appended content is followed.
func TestParseAppendAfterNoTrailingNewline(t *testing.T) {
	tmpdir := t.TempDir()
	logFileName := filepath.Join(tmpdir, "log")
	fh, err := os.Create(logFileName)
	require.NoError(t, err, "failed to create log file")

	// write a line without a trailing newline
	previousMsg := fmt.Sprintf("msg msg %08d\n", 7)
	previousMsg += fmt.Sprintf("msg msg %08d\n", 8)
	previousMsg += fmt.Sprintf("msg msg %08d\n", 9)
	msgNoNLBefore := "msg "
	_, err = fh.WriteString(previousMsg + msgNoNLBefore)
	require.NoError(t, err, "failed to write initial content to log file")
	_ = fh.Sync()

	// First parse: should read the existing line (even without newline)
	buf := bytes.NewBufferString("")
	parser := &testParser{buf: buf}
	fp := &Parser{
		WorkDir:  tmpdir,
		Callback: parser,
		Silent:   true,
	}
	r, err := fp.Parse("logPosNoNL", logFileName)
	require.NoError(t, err, "failed to parse log file with no trailing newline")

	out := parser.Slurp().String()
	require.Equal(t, previousMsg, out, "first read output does not match expected")
	require.Len(t, r, 1, "first result len must be 1")

	// Append new content (with newline) to the same file
	fh, err = os.OpenFile(logFileName, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err, "failed to open log file for appending")

	msgNoNLAfter := fmt.Sprintf("msg %08d\n", 10)
	msgAppend := fmt.Sprintf("msg msg %08d\n", 11)
	_, err = fh.WriteString(msgNoNLAfter + msgAppend)
	require.NoError(t, err, "failed to write appended content to log file")
	fh.Close()

	// Second parse using same pos file name: should read only appended content
	buf2 := bytes.NewBufferString("")
	parser2 := &testParser{buf: buf2}
	fp2 := &Parser{
		WorkDir:  tmpdir,
		Callback: parser2,
		Silent:   true,
	}
	r2, err := fp2.Parse("logPosNoNL", logFileName)
	require.NoError(t, err, "failed to parse log file after appending content")

	out2 := parser2.Slurp().String()
	require.Equal(t, msgNoNLBefore+msgNoNLAfter+msgAppend, out2, "second read output does not match expected")
	require.Len(t, r2, 1, "second result len must be 1")
}

// Test a single line longer than DefaultStartBufSize is read properly
func TestParseSingleLongLine(t *testing.T) {
	tmpdir := t.TempDir()
	logFileName := filepath.Join(tmpdir, "log")
	fh, err := os.Create(logFileName)
	require.NoError(t, err, "failed to create log file")
	// create a single long line > DefaultStartBufSize
	longLen := DefaultStartBufSize + 100
	data := bytes.Repeat([]byte("A"), longLen)
	// ensure newline at end so Scanner treats it as a line
	_, err = fh.Write(data)
	require.NoError(t, err, "failed to write long line to log file")
	_, err = fh.WriteString("\n")
	require.NoError(t, err, "failed to write newline to log file")
	fh.Close()

	buf := bytes.NewBufferString("")
	parser := &testParser{buf: buf}
	fp := &Parser{
		WorkDir:  tmpdir,
		Callback: parser,
		Silent:   true,
	}
	r, err := fp.Parse("logPosLong", logFileName)
	require.NoError(t, err, "failed to parse log file with single long line")
	out := parser.Slurp().String()
	expected := string(data) + "\n"
	require.Equal(t, expected, out, "read output does not match expected")
	require.Len(t, r, 1, "result len must be 1")
	require.Equal(t, 1, r[0].Rows, "result[0].Rows must be 1")
	require.Equal(t, int64(len(expected)), r[0].EndPos-r[0].StartPos, "r[0].EndPos - r[0].StartPos must be len(expected)")
}

// Test rotate: old (archived) file's last line has no trailing newline
// and should still be read after rotation.
func TestRotateReadOldFileWithNoTrailingNewline(t *testing.T) {
	tmpdir := t.TempDir()
	logFileName := filepath.Join(tmpdir, "log")
	fh, err := os.Create(logFileName)
	require.NoError(t, err, "failed to create log file")

	// write first line with newline and parse to set pos file
	msg1 := fmt.Sprintf("msg msg %08d\n", 30)
	_, err = fh.WriteString(msg1)
	require.NoError(t, err, "failed to write first line to log file")
	_ = fh.Sync()

	buf := bytes.NewBufferString("")
	parser := &testParser{buf: buf}
	fp := &Parser{WorkDir: tmpdir, Callback: parser, Silent: true}
	r, err := fp.Parse("logPosRotateNoNL", logFileName)
	require.NoError(t, err, "failed to parse log file after first write")
	require.Len(t, r, 1, "initial parse result len must be 1")

	// append a line WITHOUT trailing newline
	msg2 := fmt.Sprintf("msg msg %08d", 31)
	fh, err = os.OpenFile(logFileName, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err, "failed to open log file for appending")
	_, err = fh.WriteString(msg2)
	require.NoError(t, err, "failed to write second line to log file")
	fh.Close()

	// rotate the log (rename to archive)
	archived := filepath.Join(tmpdir, "log.1")
	err = os.Rename(logFileName, archived)
	require.NoError(t, err, "failed to rotate log file")

	// create a new log file and write another line
	fh, err = os.Create(logFileName)
	require.NoError(t, err, "failed to create new log file")
	msg3 := fmt.Sprintf("msg msg %08d\n", 32)
	_, err = fh.WriteString(msg3)
	require.NoError(t, err, "failed to write third line to log file")
	fh.Close()

	// parse again: it should find the archived file and read msg2 (no newline)
	buf2 := bytes.NewBufferString("")
	parser2 := &testParser{buf: buf2}
	fp2 := &Parser{WorkDir: tmpdir, Callback: parser2, Silent: true}
	r2, err := fp2.Parse("logPosRotateNoNL", logFileName)
	require.NoError(t, err, "failed to parse log file after rotation")

	out := parser2.Slurp().String()
	expected := msg2 + "\n" + msg3
	require.Equal(t, expected, out, "rotate read output does not match expected")
	require.Len(t, r2, 2, "rotate result len must be 2")
	require.Equal(t, 1, r2[0].Rows, "rotate result[0].Rows must be 1")
	require.Equal(t, 1, r2[1].Rows, "rotate result[1].Rows must be 1")
}

func TestTruncated(t *testing.T) {
	tmpdir := t.TempDir()
	logFileName := filepath.Join(tmpdir, "truncate-log")
	fh, err := os.Create(logFileName)
	require.NoError(t, err, "failed to create log file")

	// write initial lines
	var lines = 10
	for i := range lines {
		msg := fmt.Sprintf("msg msg %08d\n", i)
		_, err = fh.WriteString(msg)
		require.NoError(t, err, "failed to write initial content to log file")
	}
	_ = fh.Sync()

	buf := bytes.NewBufferString("")
	parser := &testParser{buf: buf}
	fp := &Parser{WorkDir: tmpdir, Callback: parser, Silent: true}
	r, err := fp.Parse("logPosTruncateNoNL", logFileName)
	require.NoError(t, err, "failed to parse log file after initial write")
	require.Equal(t, 1, len(r), "initial parse result len must be equal to 1")
	require.Equal(t, lines, r[0].Rows, "initial parse result rows must be equal to lines")
	fh.Close()

	// truncate the log
	err = os.Truncate(logFileName, 0)
	require.NoError(t, err, "failed to truncate log file")
	// reopen and write a new line
	fh, err = os.OpenFile(logFileName, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err, "failed to open log file for appending")

	msg3 := fmt.Sprintf("msg msg %08d\n", 32)
	_, err = fh.WriteString(msg3)
	require.NoError(t, err, "failed to write new line to log file")
	fh.Close()

	// parse again: it should be truncated and read only the new line
	buf2 := bytes.NewBufferString("")
	parser2 := &testParser{buf: buf2}
	fp2 := &Parser{WorkDir: tmpdir, Callback: parser2, Silent: false}
	r2, err := fp2.Parse("logPosTruncateNoNL", logFileName)
	require.NoError(t, err, "failed to parse log file after truncation")

	out := parser2.Slurp().String()
	expected := msg3
	require.Equal(t, expected, out, "truncated read output does not match expected")
	require.Len(t, r2, 1, "truncated result len must be 1")
	require.Equal(t, 1, r2[0].Rows, "truncated result[0].Rows must be 1")
}
