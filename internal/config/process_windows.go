//go:build windows

package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configureCommandProcess(cmd *exec.Cmd) {}

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
	job, err := createKillOnCloseJob()
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

func createKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func (p *commandProcess) Terminate() {
	if p.job != 0 {
		_ = windows.TerminateJobObject(p.job, 1)
	}
	if p.cmd.Process == nil {
		return
	}
	// Fallback for descendants spawned before the job assignment or when
	// job creation failed.
	taskkill := taskkillPath()
	_ = exec.Command(taskkill, "/T", "/F", "/PID", strconv.Itoa(p.cmd.Process.Pid)).Run()
	_ = p.cmd.Process.Kill()
}

// Close releases the job handle; KILL_ON_JOB_CLOSE reaps any process still
// assigned to the job.
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
