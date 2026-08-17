package builder

import "runtime"

// CompiledBinaryName is what every compiled runtime packages its executable as.
//
// The name is fixed rather than taken from the crate so the runner needs no
// manifest to know what to exec: the code layer always contains one file called
// this. It carries the host's extension, because Windows decides what is
// executable by extension and the host that builds is the host that runs.
var CompiledBinaryName = "app" + executableExtension()

func executableExtension() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
