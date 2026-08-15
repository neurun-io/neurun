package build

import (
	"fmt"
	"strings"
	"time"
)

// The layers every runtime knows. A build may carry others later; these two are
// the ones a runner looks for by name.
const (
	LayerCode    = "code"
	LayerInstall = "install"
)

// Artifact is one stored ZIP. Name is what it is to the runtime — the layer a
// runner unpacks and looks in — and is unique within its build, so a build can
// carry as many as a toolchain has reason to produce.
type Artifact struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"size_bytes"`
	SHA256     string    `json:"sha256"`
	StorageKey string    `json:"storage_key"`
	CreatedAt  time.Time `json:"created_at"`
}

// StorageKeyFor addresses a layer by the build that made it.
func StorageKeyFor(buildID, artifactID string) string {
	return buildID + "/" + artifactID + ".zip"
}

func ValidateArtifact(record Artifact) error {
	if err := ValidateIdentifier("artifact_id", record.ID); err != nil {
		return err
	}
	if err := validateLayerName(record.Name); err != nil {
		return err
	}
	if record.SizeBytes < 0 || len(record.SHA256) != 64 || record.CreatedAt.IsZero() {
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

// validateLayerName keeps a name usable as the directory a runner unpacks it
// into, since that is what it becomes.
func validateLayerName(name string) error {
	if name == "" || len(name) > 32 {
		return fmt.Errorf("%w: artifact name must contain 1 to 32 bytes", ErrInvalid)
	}
	for _, character := range name {
		if character == '-' || character == '_' ||
			(character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') {
			continue
		}
		return fmt.Errorf(
			"%w: artifact name must be lowercase letters, digits, dash or underscore",
			ErrInvalid,
		)
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "_") {
		return fmt.Errorf("%w: artifact name is malformed", ErrInvalid)
	}
	return nil
}
