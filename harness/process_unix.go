//go:build unix

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// killProcessGroupOnCancel starts cmd in its own process group and, on cancel, kills the
// whole group, including any server a test started.
func killProcessGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 10 * time.Second
}

// killLeftovers kills anything still running in cmd's process group after it exited.
func killLeftovers(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // ESRCH just means nothing was left
	}
}
