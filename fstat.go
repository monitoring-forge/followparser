package followparser

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type fStat struct {
	Inode uint64
	Dev   uint64
	Size  int64
}

// fileStat retrieves the file statistics for the specified filename, returning an fStat struct containing the inode, device, and size information.
func fileStat(filename string) (*fStat, error) {
	s, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	s2 := s.Sys().(*syscall.Stat_t)
	if s2 == nil {
		return nil, fmt.Errorf("could not get inode")
	}
	return &fStat{
		Inode: s2.Ino,
		Dev:   uint64(s2.Dev),
		Size:  s.Size(),
	}, nil
}

// isNotRotated checks if the file represented by fstat has not been rotated compared to lastFstat.
func (fstat *fStat) isNotRotated(lastFstat *fStat) bool {
	if lastFstat == nil {
		return true
	}
	return lastFstat.Inode == 0 || lastFstat.Dev == 0 || (fstat.Inode == lastFstat.Inode && fstat.Dev == lastFstat.Dev)
}

// searchFileByInode searches for a file in the specified directory that matches the given fStat (inode and device).
func (fstat *fStat) searchFileByInode(d string) (string, error) {
	files, err := os.ReadDir(d)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		s, err := fileStat(filepath.Join(d, file.Name()))
		if err != nil {
			continue
		}
		if s.Inode == fstat.Inode && s.Dev == fstat.Dev {
			return filepath.Join(d, file.Name()), nil
		}
	}
	return "", fmt.Errorf("there is no file by inode:%d in %s", fstat.Inode, d)
}
