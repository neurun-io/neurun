package builder

// CompiledBinaryName is what every compiled runtime packages its executable as.
//
// The name is fixed rather than taken from the entrypoint so the runner needs no
// manifest to know what to exec: the entrypoint selects which target to build,
// and the layer always contains one file called this.
const CompiledBinaryName = "handler"
