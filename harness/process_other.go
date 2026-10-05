//go:build !unix

package main

import (
	"os/exec"
	"time"
)

func killProcessGroupOnCancel(cmd *exec.Cmd) { cmd.WaitDelay = 10 * time.Second }

func killLeftovers(*exec.Cmd) {}
