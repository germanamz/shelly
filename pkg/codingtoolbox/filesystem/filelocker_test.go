package filesystem

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileLocker_ConcurrentWritesSerialized(t *testing.T) {
	fl := NewFileLocker()
	var counter int64
	var maxConcurrent int64
	var current int64

	var wg sync.WaitGroup

	for range 100 {
		wg.Go(func() {
			fl.Lock("/tmp/test.txt")
			defer fl.Unlock("/tmp/test.txt")

			c := atomic.AddInt64(&current, 1)

			// Track peak concurrency inside the lock.
			for {
				old := atomic.LoadInt64(&maxConcurrent)
				if c <= old || atomic.CompareAndSwapInt64(&maxConcurrent, old, c) {
					break
				}
			}

			atomic.AddInt64(&counter, 1)
			atomic.AddInt64(&current, -1)
		})
	}

	wg.Wait()
	assert.Equal(t, int64(100), counter)
	assert.Equal(t, int64(1), maxConcurrent, "only one goroutine should hold the lock at a time")
}

func TestFileLocker_LockPairSamePath(t *testing.T) {
	fl := NewFileLocker()

	// Should not deadlock — locks once, unlocks once.
	fl.LockPair("/a", "/a")
	fl.UnlockPair("/a", "/a")
}

func TestFileLocker_LockPairCrossedPaths(t *testing.T) {
	fl := NewFileLocker()
	done := make(chan struct{})

	go func() {
		fl.LockPair("/b", "/a")
		fl.UnlockPair("/b", "/a")
		close(done)
	}()

	fl.LockPair("/a", "/b")
	fl.UnlockPair("/a", "/b")

	<-done
}

func TestFileLocker_UnlockPairReverseOrder(t *testing.T) {
	fl := NewFileLocker()

	// Lock /a then /b (sorted order).
	fl.LockPair("/a", "/b")

	// After UnlockPair, both should be acquirable again.
	// This also verifies that passing args in swapped order still works.
	fl.UnlockPair("/b", "/a")

	// Both paths should now be unlocked — lock them individually to verify.
	done := make(chan struct{}, 2)

	go func() {
		fl.Lock("/a")
		fl.Unlock("/a")
		done <- struct{}{}
	}()

	go func() {
		fl.Lock("/b")
		fl.Unlock("/b")
		done <- struct{}{}
	}()

	<-done
	<-done
}

func TestFileLocker_IndependentPathsNotBlocked(t *testing.T) {
	fl := NewFileLocker()
	var wg sync.WaitGroup

	fl.Lock("/x")

	wg.Go(func() {
		fl.Lock("/y")
		fl.Unlock("/y")
	})

	// /y should not be blocked by /x.
	wg.Wait()
	fl.Unlock("/x")
}
