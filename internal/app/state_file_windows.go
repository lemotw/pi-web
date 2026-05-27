//go:build windows

package app

import "os"

// lockStateFile is a no-op on Windows (unsupported platform).
func lockStateFile(_ *os.File) error { return nil }
