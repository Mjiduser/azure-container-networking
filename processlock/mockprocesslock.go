package processlock

import (
	"github.com/pkg/errors"
)

// ErrMockFileLock - mock filelock error
var ErrMockFileLock = errors.New("Mock filelock error")

type mockFileLock struct {
	fail bool
}

func NewMockFileLock(fail bool) ProcessLockInterface {
	return &mockFileLock{
		fail: fail,
	}
}

func (l *mockFileLock) AcquireLock() error {
	if l.fail {
		return ErrMockFileLock
	}

	return nil
}

func (l *mockFileLock) ReleaseLock() error {
	if l.fail {
		return ErrMockFileLock
	}

	return nil
}
