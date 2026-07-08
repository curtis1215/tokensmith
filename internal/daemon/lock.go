package daemon

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// AcquireLock writes the current PID to path, providing single-instance
// protection. It fails if a live process already holds the lock; a stale lock
// (whose PID is no longer running) is stolen. The returned func releases it.
func AcquireLock(path string) (func() error, error) {
	if data, err := os.ReadFile(path); err == nil {
		if pid, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil && processAlive(pid) {
			return nil, errors.New("daemon: already running (pid " + strconv.Itoa(pid) + ")")
		}
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return nil, err
	}
	return func() error { return os.Remove(path) }, nil
}

// processAlive reports whether a process with the given pid is running.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
