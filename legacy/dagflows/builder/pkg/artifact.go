package pkg

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/dagflows/builder/internal/domain"
)

const zipMediaType = "application/zip"

type Artifact struct {
	Kind      domain.ArtifactKind
	Name      string
	Path      string
	SHA256    string
	SizeBytes int64
	MediaType string
}

func FileArtifact(kind domain.ArtifactKind, name, path, mediaType string) (Artifact, error) {
	sha, size, err := hashFile(path)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{
		Kind:      kind,
		Name:      name,
		Path:      path,
		SHA256:    sha,
		SizeBytes: size,
		MediaType: mediaType,
	}, nil
}

func hashFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}
