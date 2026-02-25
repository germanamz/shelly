package filesystem

import "sync"

// FileLocker provides per-path mutual exclusion for filesystem operations.
// It lazily allocates a mutex for each path on first use.
type FileLocker struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewFileLocker creates a new FileLocker.
func NewFileLocker() *FileLocker {
	return &FileLocker{
		locks: make(map[string]*sync.Mutex),
	}
}

func (fl *FileLocker) getMutex(path string) *sync.Mutex {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	m, ok := fl.locks[path]
	if !ok {
		m = &sync.Mutex{}
		fl.locks[path] = m
	}

	return m
}

// Lock acquires the mutex for the given path.
func (fl *FileLocker) Lock(path string) {
	fl.getMutex(path).Lock()
}

// Unlock releases the mutex for the given path.
func (fl *FileLocker) Unlock(path string) {
	fl.getMutex(path).Unlock()
}

// LockPair acquires mutexes for two paths in a consistent order to avoid
// deadlocks. If both paths are the same, only one lock is acquired.
func (fl *FileLocker) LockPair(p1, p2 string) {
	if p1 == p2 {
		fl.Lock(p1)
		return
	}

	// Always lock in sorted order to prevent deadlocks.
	if p1 > p2 {
		p1, p2 = p2, p1
	}

	fl.Lock(p1)
	fl.Lock(p2)
}

// UnlockPair releases mutexes for two paths. If both paths are the same,
// only one lock is released.
func (fl *FileLocker) UnlockPair(p1, p2 string) {
	if p1 == p2 {
		fl.Unlock(p1)
		return
	}

	// Sort to match LockPair order, then unlock in reverse.
	if p1 > p2 {
		p1, p2 = p2, p1
	}

	fl.Unlock(p2)
	fl.Unlock(p1)
}
