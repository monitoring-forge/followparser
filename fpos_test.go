package followparser

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func init() {
	log.SetOutput(io.Discard)
}
func TestPosFileRead(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pos_file_test")
	require.NoError(t, err, "failed to create temp directory")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	tests := []struct {
		name          string
		content       *fPos
		expectedPos   int64
		expectedTime  float64
		expectedFStat *fStat
		expectedError bool
	}{
		{
			name: "valid file",
			content: &fPos{
				Pos:   123,
				Time:  float64(time.Now().Unix()),
				Inode: 1,
				Dev:   2,
			},
			expectedPos:  123,
			expectedTime: 0.0, // Ideally time should be compared within a range
			expectedFStat: &fStat{
				Inode: 1,
				Dev:   2,
				Size:  0,
			},
			expectedError: false,
		},
		{
			name:          "file does not exist",
			content:       nil,
			expectedPos:   0,
			expectedTime:  0,
			expectedFStat: nil,
			expectedError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filename := path.Join(tmpDir, tc.name)
			if tc.content != nil {
				fileContent, _ := json.Marshal(tc.content)
				errWF := os.WriteFile(filename, fileContent, 0666)
				require.NoError(t, errWF, "failed to write test content to file")
			}

			pf := newPosFile(filename)
			pos, _, fstat, err := pf.read()
			require.Equal(t, tc.expectedError, err != nil, "error expectation mismatch")
			require.Equal(t, tc.expectedPos, pos, "pos expectation mismatch")
			if fstat != nil && tc.expectedFStat != nil {
				require.Equal(t, tc.expectedFStat.Inode, fstat.Inode, "inode expectation mismatch")
				require.Equal(t, tc.expectedFStat.Dev, fstat.Dev, "dev expectation mismatch")
			}

			// Validate duration within a reasonable time range if needed
		})
	}
}

func TestPosFileWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pos_file_test")
	require.NoError(t, err, "failed to create temp directory")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	tests := []struct {
		name          string
		pos           int64
		fstat         *fStat
		expectedError bool
	}{
		{
			name: "valid write",
			pos:  456,
			fstat: &fStat{
				Inode: 3,
				Dev:   4,
			},
			expectedError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filename := path.Join(tmpDir, "pos_file.json")
			pf := newPosFile(filename)
			err := pf.write(tc.pos, tc.fstat)
			require.Equal(t, tc.expectedError, err != nil, "error expectation mismatch")

			if !tc.expectedError {
				content, _ := os.ReadFile(filename)
				readFPos := &fPos{}
				err := json.Unmarshal(content, readFPos)
				require.NoError(t, err, "failed to unmarshal content")

				require.Equal(t, tc.pos, readFPos.Pos, "pos expectation mismatch")
				require.Equal(t, tc.fstat.Inode, readFPos.Inode, "inode expectation mismatch")
				require.Equal(t, tc.fstat.Dev, readFPos.Dev, "dev expectation mismatch")

				// Validate time within a reasonable range if necessary
			}
		})
	}
}

func TestPosFileConcurrentReadAndWrite(t *testing.T) {
	t.Parallel()
	tmpDir, err := os.MkdirTemp("", "pos_file_test")
	require.NoError(t, err, "failed to create temp directory")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	filename := path.Join(tmpDir, "pos_file.json")
	pf := newPosFile(filename)

	initialPos := int64(789)
	initialFStat := &fStat{
		Inode: 5,
		Dev:   6,
	}
	err = pf.write(initialPos, initialFStat)
	require.NoError(t, err, "failed to write initial data")

	iterations := 100
	var mu sync.Mutex
	ioErrs := make([]error, 0)
	done := make(chan struct{})
	go func() {
		for range iterations {
			_, _, _, err := pf.read()
			if err != nil {
				mu.Lock()
				ioErrs = append(ioErrs, err)
				mu.Unlock()
			}
			time.Sleep(time.Millisecond)
		}
		close(done)
	}()

	for i := range iterations {
		pos := int64(1000 + i)
		err := pf.write(pos, initialFStat)
		if err != nil {
			mu.Lock()
			ioErrs = append(ioErrs, err)
			mu.Unlock()
		}
		time.Sleep(time.Millisecond)
	}

	<-done
	require.Empty(t, ioErrs, "expected no errors during concurrent read/write")
}
