package processlock

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileLock(t *testing.T) {
	tests := []struct {
		name           string
		flock          ProcessLockInterface
		wantErr        bool
		deleteLockfile bool
		wantErrMsg     string
		lockfileName   string
	}{
		{
			name:           "Create new file and acquire Lock",
			flock:          &fileLock{filePath: "testfiles/newaz.lock"},
			wantErr:        false,
			deleteLockfile: true,
			lockfileName:   "testfiles/newaz.lock",
		},
		{
			name:         "acquire Lock on existing file",
			flock:        &fileLock{filePath: "testfiles/azure.lock"},
			lockfileName: "testfiles/azure.lock",
			wantErr:      false,
		},
		{
			name:         "acquire Lock on existing file after releasing",
			flock:        &fileLock{filePath: "testfiles/azure.lock"},
			lockfileName: "testfiles/azure.lock",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flock.AcquireLock()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				b, errRead := ioutil.ReadFile(tt.lockfileName)
				require.NoError(t, errRead, "Got error reading lockfile:%v", errRead)
				pidStr := string(b)
				pid, _ := strconv.Atoi(pidStr)
				require.Equal(t, os.Getpid(), pid, "Expected pid %d but got %d", os.Getpid(), pid)
				err = tt.flock.ReleaseLock()
				require.NoError(t, err)
				err = tt.flock.ReleaseLock()
				require.NoError(t, err, "Calling Release lock again should not throw error for already released lock")
			}
			if tt.deleteLockfile {
				os.Remove(tt.lockfileName)
			}
		})
	}
}

func TestReleaseFileLockError(t *testing.T) {
	tests := []struct {
		name       string
		flock      ProcessLockInterface
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "Release file lock without acquring it",
			flock:      &fileLock{filePath: "testfiles/newaz.lock"},
			wantErr:    true,
			wantErrMsg: ErrInvalidFile.Error(),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flock.ReleaseLock()
			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, tt.wantErrMsg, err.Error(), "Expected:%s but got:%s", tt.wantErrMsg, err.Error())
			}
		})
	}
}
