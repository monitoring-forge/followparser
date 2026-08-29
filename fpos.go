package followparser

import (
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/monitoring-forge/saferio"
)

type fPos struct {
	Pos   int64   `json:"pos"`
	Time  float64 `json:"time"`
	Inode uint64  `json:"inode"`
	Dev   uint64  `json:"dev"`
}

type posFile struct {
	workDir  string
	filename string
}

func newPosFile(workdir, filename string) *posFile {
	return &posFile{
		workDir:  workdir,
		filename: filename,
	}
}

func (pf *posFile) read() (int64, float64, *fStat, error) {
	s, err := saferio.Stat(pf.workDir, pf.filename)
	if err != nil || s.Size() == 0 {
		return 0, 0, nil, nil
	}

	fp := fPos{}
	err = retry.Do(
		func() error {
			err := saferio.ReadJSON(pf.workDir, pf.filename, &fp)
			if err != nil {
				return err
			}
			return nil
		},
		retry.Attempts(3),
		retry.DelayType(retry.FixedDelay),
		retry.Delay(100*time.Millisecond),
	)

	if err != nil {
		return 0, 0, nil, err
	}
	duration := float64(time.Now().Unix()) - fp.Time
	return fp.Pos,
		duration,
		&fStat{
			Inode: fp.Inode,
			Dev:   fp.Dev,
			Size:  0,
		},
		nil
}

func (pf *posFile) write(pos int64, fstat *fStat) error {
	fp := fPos{
		Pos:   pos,
		Time:  float64(time.Now().Unix()),
		Inode: fstat.Inode,
		Dev:   fstat.Dev,
	}
	return saferio.WriteJSON(pf.workDir, pf.filename, fp)
}
