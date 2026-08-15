package build

import (
	"fmt"
	"time"
)

const (
	ArtifactSource       = "deployment_source"
	ArtifactInstallLayer = "install_layer"
	ArtifactCodeLayer    = "code_layer"
)

// Artifact is immutable payload metadata. StorageKey is the blob handle
// builders and workers open; it names internal storage topology, so the API
// projects artifacts into a response type rather than serving this one.
type Artifact struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Name       string    `json:"name"`
	MediaType  string    `json:"media_type"`
	SizeBytes  int64     `json:"size_bytes"`
	SHA256     string    `json:"sha256"`
	StorageKey string    `json:"storage_key"`
	CreatedAt  time.Time `json:"created_at"`
}

// ValidateArtifact checks payload metadata. An empty expectedKind accepts any
// kind, which is what a build's own layers are checked against.
func ValidateArtifact(record Artifact, expectedKind string) error {
	if err := ValidateIdentifier("artifact_id", record.ID); err != nil {
		return err
	}
	if expectedKind != "" && record.Kind != expectedKind {
		return fmt.Errorf("%w: artifact kind is invalid", ErrInvalid)
	}
	if record.Kind == "" || record.Name == "" || record.MediaType == "" ||
		record.SizeBytes < 0 || len(record.SHA256) != 64 ||
		record.CreatedAt.IsZero() {
		return fmt.Errorf("%w: artifact metadata is incomplete", ErrInvalid)
	}
	for _, character := range record.SHA256 {
		if !((character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'f')) {
			return fmt.Errorf("%w: artifact digest is invalid", ErrInvalid)
		}
	}
	if err := ValidateStorageKey(record.StorageKey); err != nil {
		return fmt.Errorf("%w: artifact storage handle: %v", ErrInvalid, err)
	}
	return nil
}
