//go:build windows

package worker

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
)

// configureProcessTree makes cancellation terminate descendants as well as
// the Python parent. taskkill is part of supported Windows installations.
func configureProcessTree(command *exec.Cmd) {
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(command.Process.Pid)).Run()
		if err != nil {
			killErr := command.Process.Kill()
			if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				return errors.Join(err, killErr)
			}
		}
		return nil
	}
}
