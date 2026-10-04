//go:build unix

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// killProcessGroupOnCancel starts cmd in its own process group and, when its context is
// cancelled, kills the whole group rather than just cmd.
func killProcessGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 10 * time.Second
}

// killLeftovers kills whatever is still running in cmd's process group after cmd exited,
// such as a process the contestant's code started in the background.
func killLeftovers(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // ESRCH just means nothing was left
	}
}
