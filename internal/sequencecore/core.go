package sequencecore

import (
	"errors"
	"strings"
	"sync"
)

var ErrNoGapLocked = errors.New("sequence row is locked")

type Key struct {
	Namespace string
	Model     string
	ID        int64
}

type standardCounter struct {
	RowNumberNext int64
	Next          int64
}

var standardCounters = struct {
	sync.Mutex
	values map[Key]standardCounter
}{values: map[Key]standardCounter{}}

var noGapLocks = struct {
	sync.Mutex
	values map[Key]*sync.Mutex
}{values: map[Key]*sync.Mutex{}}

func NextNumber(key Key, implementation string, rowNumberNext int64, increment int64) (number int64, next int64, mutateRow bool, err error) {
	if rowNumberNext == 0 {
		rowNumberNext = 1
	}
	if increment == 0 {
		increment = 1
	}
	if normalizedImplementation(implementation) == "no_gap" {
		lock := noGapLock(key)
		if !lock.TryLock() {
			return 0, 0, false, ErrNoGapLocked
		}
		defer lock.Unlock()
		return rowNumberNext, rowNumberNext + increment, true, nil
	}
	standardCounters.Lock()
	defer standardCounters.Unlock()
	counter := standardCounters.values[key]
	if counter.Next == 0 || counter.RowNumberNext != rowNumberNext {
		counter = standardCounter{RowNumberNext: rowNumberNext, Next: rowNumberNext}
	}
	number = counter.Next
	counter.Next += increment
	standardCounters.values[key] = counter
	return number, counter.Next, false, nil
}

func normalizedImplementation(value string) string {
	switch strings.TrimSpace(value) {
	case "no_gap":
		return "no_gap"
	default:
		return "standard"
	}
}

func noGapLock(key Key) *sync.Mutex {
	noGapLocks.Lock()
	defer noGapLocks.Unlock()
	lock := noGapLocks.values[key]
	if lock == nil {
		lock = &sync.Mutex{}
		noGapLocks.values[key] = lock
	}
	return lock
}

func LockNoGapForTesting(key Key) func() {
	lock := noGapLock(key)
	lock.Lock()
	return lock.Unlock
}

func ResetForTesting() {
	standardCounters.Lock()
	standardCounters.values = map[Key]standardCounter{}
	standardCounters.Unlock()
	noGapLocks.Lock()
	noGapLocks.values = map[Key]*sync.Mutex{}
	noGapLocks.Unlock()
}
