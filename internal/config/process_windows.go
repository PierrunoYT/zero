//go:build windows

package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// configureCommandProcess starts the process suspended so it can be
// assigned to the job object before its main thread (and therefore any
// code it runs) executes. Without this, a fast command can spawn and
// detach a descendant before AssignProcessToJobObject runs, letting that
// descendant escape the job and survive termination.
func configureCommandProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
}

// commandProcess tracks a started provider command so its entire process
// tree can be terminated on timeout. It prefers a job object: taskkill /T
// walks the tree by parent PID in user space and can miss descendants,
// letting them run to completion while Wait blocks on inherited pipes.
type commandProcess struct {
	cmd *exec.Cmd
	job windows.Handle
}

func attachCommandProcess(cmd *exec.Cmd) *commandProcess {
	proc := &commandProcess{cmd: cmd}
	if cmd.Process == nil {
		return proc
	}
	// The main thread is suspended (see configureCommandProcess); resume it
	// once we're done attaching, however that turns out. If resuming fails
	// outright (Toolhelp/OpenThread/ResumeThread errors are otherwise
	// swallowed), the process would sit suspended forever and never exit on
	// its own, making the failure look like an ordinary 5s provider-command
	// timeout. Terminate it immediately instead so the real cause surfaces
	// quickly and no suspended process is left behind.
	defer func() {
		if !resumeMainThread(cmd.Process.Pid) {
			proc.Terminate()
		}
	}()

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return proc
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return proc
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	if err := windows.AssignProcessToJobObject(job, handle); err != nil {
		_ = windows.CloseHandle(job)
		return proc
	}
	proc.job = job
	return proc
}

// resumeMainThread resumes every (assumed suspended) thread of pid and
// reports whether at least one was actually resumed. Callers must treat a
// false result as a failed resume, not a benign no-op: a suspended process
// that's never woken up will neither exit nor produce output on its own.
func resumeMainThread(pid int) bool {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()

	var entry windows.ThreadEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	resumed := false
	for err := windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(pid) {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			continue
		}
		if _, err := windows.ResumeThread(thread); err == nil {
			resumed = true
		}
		_ = windows.CloseHandle(thread)
	}
	return resumed
}

func (p *commandProcess) Terminate() {
	if p.job != 0 {
		// A job handle means the process tree was contained from launch, so
		// TerminateJobObject alone kills every member atomically. Don't also
		// fall through to the taskkill/PID path below: by the time
		// Terminate runs on the ErrWaitDelay path, cmd.Wait has already
		// reaped the root process, so its PID may have been reused by an
		// unrelated process, and forcing /T /PID against a reused PID would
		// kill the wrong tree.
		_ = windows.TerminateJobObject(p.job, 1)
		return
	}
	if p.cmd.Process == nil {
		return
	}
	// Fallback for the rare case where job creation or assignment failed:
	// there's no containment, so the PID is the best signal available even
	// though it carries the same reuse risk noted above. Known limitation:
	// unlike the job-object path, this can't reach a descendant that has
	// already reparented away from the tree taskkill /T walks. This is not
	// a regression versus pre-job-object behavior (taskkill /T was the only
	// mechanism then too); closing it fully would require containment that
	// doesn't depend on job objects and is tracked as a follow-up.
	taskkill := taskkillPath()
	_ = exec.Command(taskkill, "/T", "/F", "/PID", strconv.Itoa(p.cmd.Process.Pid)).Run()
	_ = p.cmd.Process.Kill()
}

// Close releases the job handle without touching any still-running
// descendants: the job carries no KILL_ON_JOB_CLOSE limit, so on the
// success path a provider command's detached helpers keep running exactly
// as they did before job objects were introduced. Descendant termination
// happens explicitly via Terminate, called only on timeout/error.
func (p *commandProcess) Close() {
	if p.job == 0 {
		return
	}
	_ = windows.CloseHandle(p.job)
	p.job = 0
}

func taskkillPath() string {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = os.Getenv("windir")
	}
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	return filepath.Join(systemRoot, "System32", "taskkill.exe")
}
