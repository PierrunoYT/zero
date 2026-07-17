//go:build windows

package config

import (
	"testing"

	"golang.org/x/sys/windows"
)

// TestResumeMainThreadReportsFailureForUnknownPID covers the contract
// attachCommandProcess relies on: when no thread belonging to pid can be
// found (here, because the pid doesn't correspond to a live process),
// resumeMainThread must report false rather than silently doing nothing,
// so a caller can react (e.g. terminate the process) instead of leaving it
// suspended indefinitely.
func TestResumeMainThreadReportsFailureForUnknownPID(t *testing.T) {
	if resumeMainThread(0) {
		t.Fatal("resumeMainThread(0) = true, want false for a pid with no threads")
	}
}

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	const stillActive = 259 // STILL_ACTIVE
	return code == stillActive
}
