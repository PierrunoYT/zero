package background

import "os/exec"

// TerminateProcess stops a background process by PID — on Windows its process
// tree; on POSIX its whole process group when the PID leads its own group (the
// invariant ConfigureChildProcessGroup establishes for processes started through
// this package), otherwise just the individual PID. The PID-only fallback is
// deliberate — signalling a non-leader's group could hit unrelated processes — but
// it means descendants of a non-leader are NOT reaped; pass a group leader to
// guarantee group termination. Exported for callers that hold a raw PID and cannot
// route through the manager, e.g. cleaning up a just-launched child whose PID could
// not be recorded.
func TerminateProcess(pid int) error {
	return terminateProcess(pid)
}

// TerminateCommand stops a started command's process tree/group and reaps the
// leader. Keeping both operations together lets platform implementations signal
// the tree before Wait can discard the leader identity needed to find it.
func TerminateCommand(cmd *exec.Cmd) error {
	return terminateCommand(cmd)
}
