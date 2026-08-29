package followparser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileStat(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("regular file", func(t *testing.T) {
		fname := filepath.Join(tmpDir, "test.log")
		content := []byte("hello world\n")
		err := os.WriteFile(fname, content, 0644)
		require.NoError(t, err, "failed to create test file")

		fs, err := fileStat(fname)
		require.NoError(t, err, "fileStat should not return error for existing file")
		require.NotNil(t, fs, "fileStat should return non-nil fStat")
		require.NotZero(t, fs.Inode, "inode should not be zero")
		require.NotZero(t, fs.Dev, "dev should not be zero")
		require.Equal(t, int64(len(content)), fs.Size, "size should match file content length")
	})

	t.Run("file does not exist", func(t *testing.T) {
		fname := filepath.Join(tmpDir, "nonexistent.log")
		fs, err := fileStat(fname)
		require.Error(t, err, "fileStat should return error for non-existent file")
		require.Nil(t, fs, "fileStat should return nil fStat for non-existent file")
	})

	t.Run("directory", func(t *testing.T) {
		dname := filepath.Join(tmpDir, "testdir")
		err := os.Mkdir(dname, 0755)
		require.NoError(t, err, "failed to create test directory")

		fs, err := fileStat(dname)
		require.NoError(t, err, "fileStat should not return error for directory")
		require.NotNil(t, fs, "fileStat should return non-nil fStat for directory")
		require.NotZero(t, fs.Inode, "inode should not be zero for directory")
	})
}

func TestFStatIsNotRotated(t *testing.T) {
	tests := []struct {
		name      string
		fstat     *fStat
		lastFstat *fStat
		expected  bool
	}{
		{
			name:      "lastFstat is nil",
			fstat:     &fStat{Inode: 1, Dev: 2},
			lastFstat: nil,
			expected:  true,
		},
		{
			name:      "same inode and dev",
			fstat:     &fStat{Inode: 1, Dev: 2},
			lastFstat: &fStat{Inode: 1, Dev: 2},
			expected:  true,
		},
		{
			name:      "different inode same dev",
			fstat:     &fStat{Inode: 2, Dev: 2},
			lastFstat: &fStat{Inode: 1, Dev: 2},
			expected:  false,
		},
		{
			name:      "same inode different dev",
			fstat:     &fStat{Inode: 1, Dev: 3},
			lastFstat: &fStat{Inode: 1, Dev: 2},
			expected:  false,
		},
		{
			name:      "lastFstat inode is zero",
			fstat:     &fStat{Inode: 1, Dev: 2},
			lastFstat: &fStat{Inode: 0, Dev: 2},
			expected:  true,
		},
		{
			name:      "lastFstat dev is zero",
			fstat:     &fStat{Inode: 1, Dev: 2},
			lastFstat: &fStat{Inode: 1, Dev: 0},
			expected:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.fstat.isNotRotated(tc.lastFstat)
			require.Equal(t, tc.expected, actual, "isNotRotated result mismatch")
		})
	}
}

func TestFStatSearchFileByInode(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("find matching file", func(t *testing.T) {
		fname := filepath.Join(tmpDir, "target.log")
		err := os.WriteFile(fname, []byte("target content\n"), 0644)
		require.NoError(t, err, "failed to create target file")

		fs, err := fileStat(fname)
		require.NoError(t, err, "failed to get file stat")

		found, err := fs.searchFileByInode(tmpDir)
		require.NoError(t, err, "searchFileByInode should find matching file")
		require.Equal(t, fname, found, "found file path should match target")
	})

	t.Run("no matching file", func(t *testing.T) {
		fs := &fStat{Inode: 999999999, Dev: 999999999}
		_, err := fs.searchFileByInode(tmpDir)
		require.Error(t, err, "searchFileByInode should return error when no matching file")
	})

	t.Run("directory does not exist", func(t *testing.T) {
		fs := &fStat{Inode: 1, Dev: 2}
		_, err := fs.searchFileByInode(filepath.Join(tmpDir, "nonexistent"))
		require.Error(t, err, "searchFileByInode should return error for non-existent directory")
	})

	t.Run("skip directories", func(t *testing.T) {
		dname := filepath.Join(tmpDir, "subdir")
		err := os.Mkdir(dname, 0755)
		require.NoError(t, err, "failed to create subdirectory")

		fs := &fStat{Inode: 999999999, Dev: 999999999}
		_, err = fs.searchFileByInode(tmpDir)
		require.Error(t, err, "searchFileByInode should not match directories")
	})

	t.Run("ignore files that cannot be stated", func(t *testing.T) {
		goodFile := filepath.Join(tmpDir, "good.log")
		err := os.WriteFile(goodFile, []byte("good content\n"), 0644)
		require.NoError(t, err, "failed to create good file")

		// searchFileByInode ignores files that fail fileStat. Create a symlink
		// to a non-existent target so that fileStat returns an error and is skipped.
		symlink := filepath.Join(tmpDir, "broken_symlink")
		err = os.Symlink(filepath.Join(tmpDir, "does_not_exist"), symlink)
		require.NoError(t, err, "failed to create broken symlink")

		fs, err := fileStat(goodFile)
		require.NoError(t, err, "failed to get good file stat")

		found, err := fs.searchFileByInode(tmpDir)
		require.NoError(t, err, "searchFileByInode should find good file while ignoring broken symlink")
		require.Equal(t, goodFile, found, "found file path should match good file")
	})
}
