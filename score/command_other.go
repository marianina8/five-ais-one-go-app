//go:build !unix

package main

import (
	"os/exec"
	"time"
)

// killProcessGroupOnCancel can only kill cmd itself on this system.
func killProcessGroupOnCancel(cmd *exec.Cmd) {
	cmd.WaitDelay = 10 * time.Second
}

// killLeftovers does nothing here: without process groups there is no way to find them.
func killLeftovers(*exec.Cmd) {}
