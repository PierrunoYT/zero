//go:build !windows

package config

import (
	"os/exec"
	"syscall"
)

func configureCommandProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

type commandProcess struct {
	cmd *exec.Cmd
}

func attachCommandProcess(cmd *exec.Cmd) *commandProcess {
	return &commandProcess{cmd: cmd}
}

func (p *commandProcess) Terminate() {
	if p.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	_ = p.cmd.Process.Kill()
}

func (p *commandProcess) Close() {}
