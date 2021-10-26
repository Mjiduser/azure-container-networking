package processlock

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/internal/lockedfile"
	"github.com/pkg/errors"
)

// ErrInvalidFile - invalid file pointer
var (
	ErrEmptyFilePath = errors.New("Empty file path")
	ErrInvalidFile   = errors.New("Invalid File pointer")
)

//nolint:revive // this naming makes sense
type ProcessLockInterface interface {
	AcquireLock() error
	ReleaseLock() error
}

type fileLock struct {
	filePath string
	file     *lockedfile.File
}

func NewFileLock(fileAbsPath string) (ProcessLockInterface, error) {
	if fileAbsPath == "" {
		return nil, ErrEmptyFilePath
	}

	//nolint:gomnd //0o664 - permission to create directory in octal
	err := os.MkdirAll(filepath.Dir(fileAbsPath), os.FileMode(0o664))
	if err != nil {
		return nil, errors.Wrap(err, "mkdir lock dir returned error")
	}

	return &fileLock{
		filePath: fileAbsPath,
	}, nil
}

func (l *fileLock) AcquireLock() error {
	var err error

	l.file, err = lockedfile.Create(l.filePath)
	if err != nil {
		return errors.Wrap(err, "Failed to acquire lock")
	}

	_, err = l.file.WriteString(strconv.Itoa(os.Getpid()))
	if err != nil {
		return errors.Wrap(err, "Write to lockfile failed")
	}

	return nil
}

func (l *fileLock) ReleaseLock() error {
	if l.file == nil {
		return ErrInvalidFile
	}

	err := l.file.Close()
	if err != nil && !strings.Contains(err.Error(), fs.ErrClosed.Error()) {
		return errors.Wrap(err, "Failed to release lock")
	}

	return nil
}
