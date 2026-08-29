package followparser

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

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
