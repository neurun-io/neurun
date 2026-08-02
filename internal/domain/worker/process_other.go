//go:build !windows && !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris

package worker

import "os/exec"

func configureProcessTree(command *exec.Cmd) {}
