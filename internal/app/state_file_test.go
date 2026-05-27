//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestStateFileFlockBlocksSecondAcquire(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")

	f1, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f1.Close()
	if err := syscall.Flock(int(f1.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("first flock should succeed: %v", err)
	}

	f2, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	err = syscall.Flock(int(f2.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != syscall.EWOULDBLOCK {
		t.Fatalf("second flock should return EWOULDBLOCK, got %v", err)
	}
}
