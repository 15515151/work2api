//go:build !windows

package selfupdate

import "os/exec"

func prepareProbe(command *exec.Cmd) {}
