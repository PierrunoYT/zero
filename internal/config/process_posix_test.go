//go:build !windows

package config

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestCommandProcessRetainsGroupIdentityAfterProviderWait(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 7")
	proc, err := startCommandProcess(cmd)
	if err != nil {
		t.Fatalf("startCommandProcess: %v", err)
	}
	defer func() {
		proc.Terminate()
		proc.Close()
	}()

	providerPID := cmd.Process.Pid
	anchorPID := proc.groupID
	if err := cmd.Wait(); err == nil {
		t.Fatal("Wait error = nil, want nonzero exit")
	}
	if anchorPID == providerPID {
		t.Fatalf("group identity reused provider PID %d", providerPID)
	}
	if !processAlive(anchorPID) {
		t.Fatalf("group leader %d exited before cleanup", anchorPID)
	}

	proc.Terminate()
	proc.Close()
	if processAlive(anchorPID) {
		t.Fatalf("group leader %d still alive after cleanup", anchorPID)
	}
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
