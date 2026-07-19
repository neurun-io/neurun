package domain

type Runtime string

const (
	RuntimePython Runtime = "python"
	RuntimeNode   Runtime = "node"
	RuntimeGo     Runtime = "go"
)

type ArtifactKind string

const (
	ArtifactInstallLayer ArtifactKind = "install_layer"
	ArtifactCodeLayer    ArtifactKind = "code_layer"
	ArtifactDeployable   ArtifactKind = "deployable"
)

type BuildRequest struct {
	AppID      string
	BuildID    string
	SourcePath string
	Runtime    Runtime
	EntryPoint string
}

type UploadedArtifact struct {
	Bucket    string
	Key       string
	SizeBytes int64
}

type Artifact struct {
	ID        string
	Kind      ArtifactKind
	Name      string
	Bucket    string
	Key       string
	SHA256    string
	SizeBytes int64
	MediaType string
}

type BuildResult struct {
	BuildID   string
	Artifacts []Artifact
}
