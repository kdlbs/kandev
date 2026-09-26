package ownershiplock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrConflict means another process currently owns a required runtime target.
var ErrConflict = errors.New("runtime state is already owned")

// ConflictError identifies the target that prevented startup.
type ConflictError struct {
	Target Target
	Owner  *OwnerRecord
}

func (e *ConflictError) Error() string {
	message := fmt.Sprintf("%s target %q is already owned by another backend", e.Target.Kind, e.Target.ResourcePath)
	if e.Owner != nil {
		message += " (" + e.Owner.String() + ")"
	}
	return message
}

func (e *ConflictError) Unwrap() error { return ErrConflict }

type heldLock struct {
	target Target
	file   *os.File
}

// Owner holds all locks acquired for one backend process. Close is idempotent
// and never removes the sidecar files; the OS lock is the ownership proof.
type Owner struct {
	mu     sync.Mutex
	held   []heldLock
	closed bool
}

// Acquire obtains every target without waiting. If any target is unavailable,
// previously acquired targets are released before the error is returned.
func Acquire(targets []Target) (*Owner, error) {
	owner := &Owner{}
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if _, exists := seen[target.LockPath]; exists {
			continue
		}
		seen[target.LockPath] = struct{}{}

		file, err := openLockFile(target.LockPath)
		if err != nil {
			_ = owner.Close()
			return nil, fmt.Errorf("open %s ownership lock %q: %w", target.Kind, target.LockPath, err)
		}
		if err := lockFile(file); err != nil {
			_ = file.Close()
			_ = owner.Close()
			if errors.Is(err, ErrConflict) {
				return nil, &ConflictError{Target: target, Owner: readOwner(target.LockPath)}
			}
			return nil, fmt.Errorf("acquire %s ownership lock %q: %w", target.Kind, target.LockPath, err)
		}
		_ = writeOwnerFn(file)
		owner.held = append(owner.held, heldLock{target: target, file: file})
	}
	return owner, nil
}

func openLockFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return openLockFilePlatform(path)
}

// Close releases every held OS lock in reverse acquisition order. The first
// release or close error is returned, while all handles are still drained.
func (o *Owner) Close() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	o.closed = true
	held := o.held
	o.held = nil
	o.mu.Unlock()

	var firstErr error
	for i := len(held) - 1; i >= 0; i-- {
		if err := unlockFile(held[i].file); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("release %s ownership lock %q: %w", held[i].target.Kind, held[i].target.LockPath, err)
		}
		if err := held[i].file.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close %s ownership lock %q: %w", held[i].target.Kind, held[i].target.LockPath, err)
		}
	}
	return firstErr
}
