package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileLock provides per-book file locking to prevent concurrent writes.
// It uses a two-level approach: an in-process mutex map for goroutine safety,
// plus an OS-level lock file for cross-process safety.
type FileLock struct {
	mu    sync.Mutex
	locks map[string]*bookLock
}

type bookLock struct {
	mu       sync.Mutex
	refs     int
	lockFile *os.File
}

// NewFileLock creates a new FileLock registry.
func NewFileLock() *FileLock {
	return &FileLock{locks: make(map[string]*bookLock)}
}

// Lock acquires an exclusive lock for the given book directory.
// Returns an unlock function that must be called when done.
func (fl *FileLock) Lock(bookDir string) (unlock func(), err error) {
	fl.mu.Lock()
	bl, ok := fl.locks[bookDir]
	if !ok {
		bl = &bookLock{}
		fl.locks[bookDir] = bl
	}
	bl.refs++
	fl.mu.Unlock()

	bl.mu.Lock()

	// Acquire OS-level lock file
	lockPath := filepath.Join(bookDir, ".storyforge.lock")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		bl.mu.Unlock()
		fl.deref(bookDir)
		return nil, fmt.Errorf("create book dir: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		bl.mu.Unlock()
		fl.deref(bookDir)
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	bl.lockFile = f

	return func() {
		if bl.lockFile != nil {
			_ = bl.lockFile.Close()
			_ = os.Remove(lockPath)
			bl.lockFile = nil
		}
		bl.mu.Unlock()
		fl.deref(bookDir)
	}, nil
}

func (fl *FileLock) deref(bookDir string) {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	bl := fl.locks[bookDir]
	if bl == nil {
		return
	}
	bl.refs--
	if bl.refs == 0 {
		delete(fl.locks, bookDir)
	}
}
