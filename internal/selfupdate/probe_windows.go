package selfupdate

import (
	"os/exec"
	"syscall"
)

func prepareProbe(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
}
