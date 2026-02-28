package filesystem

import "sync"

// lockEntry holds a per-path mutex and a reference count so the entry can be
// removed from the map when no goroutine is using it.
type lockEntry struct {
	mu   sync.Mutex
	refs int
}

// FileLocker provides per-path mutual exclusion for filesystem operations.
// It lazily allocates a mutex for each path on first use and removes the
// entry when the last holder unlocks.
type FileLocker struct {
	mu    sync.Mutex
	locks map[string]*lockEntry
}

// NewFileLocker creates a new FileLocker.
func NewFileLocker() *FileLocker {
	return &FileLocker{
		locks: make(map[string]*lockEntry),
	}
}

func (fl *FileLocker) acquire(path string) *lockEntry {
	fl.mu.Lock()
	e, ok := fl.locks[path]
	if !ok {
		e = &lockEntry{}
		fl.locks[path] = e
	}
	e.refs++
	fl.mu.Unlock()

	return e
}

// Lock acquires the mutex for the given path.
func (fl *FileLocker) Lock(path string) {
	fl.acquire(path).mu.Lock()
}

// Unlock releases the mutex for the given path and removes the internal entry
// when no other goroutine holds a reference.
func (fl *FileLocker) Unlock(path string) {
	fl.mu.Lock()
	e, ok := fl.locks[path]
	if !ok {
		fl.mu.Unlock()
		return
	}
	e.refs--
	if e.refs == 0 {
		delete(fl.locks, path)
	}
	fl.mu.Unlock()

	e.mu.Unlock()
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
