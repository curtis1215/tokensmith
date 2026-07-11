//go:build unix

package dailyusage

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// ErrLockTimeout is returned when the advisory lock cannot be acquired
// within the configured deadline.
var ErrLockTimeout = errors.New("dailyusage: lock timeout")

// acquireFileLock opens path (creating it with 0600 if needed) and takes an
// exclusive non-blocking flock, retrying every 10ms until deadline.
// The returned release function unlocks and closes the file.
func acquireFileLock(path string, timeout time.Duration) (func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("dailyusage: open lock: %w", err)
	}
	// Ensure owner-only permissions even if the file already existed.
	_ = f.Chmod(0o600)

	deadline := time.Now().Add(timeout)
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() error {
				_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
				return f.Close()
			}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = f.Close()
			return nil, fmt.Errorf("dailyusage: flock: %w", err)
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, ErrLockTimeout
		}
		time.Sleep(10 * time.Millisecond)
	}
}
