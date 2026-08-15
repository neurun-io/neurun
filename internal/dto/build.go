package dto

import (
	"time"

	"github.com/neurun-io/neurun/internal/domain/build"
)

// ArtifactResponse drops StorageKey, the internal blob handle. Serving the
// domain artifact directly would publish storage topology.
type ArtifactResponse struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	MediaType string    `json:"media_type"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

type BuildResponse struct {
	ID           string             `json:"id"`
	Runtime      build.Runtime      `json:"runtime"`
	EntryPoint   string             `json:"entrypoint"`
	SourceSHA256 string             `json:"source_sha256"`
	Artifacts    []ArtifactResponse `json:"artifacts"`
	CreatedAt    time.Time          `json:"created_at"`
	// Where it came from, answered when a build is served on its own. Nested
	// under the deployment that made it, both are already known.
	DeploymentID string `json:"deployment_id,omitempty"`
	AppID        string `json:"app_id,omitempty"`
}

func NewArtifactResponse(record build.Artifact) ArtifactResponse {
	return ArtifactResponse{
		ID: record.ID, Kind: record.Kind, Name: record.Name,
		MediaType: record.MediaType, SizeBytes: record.SizeBytes,
		SHA256: record.SHA256, CreatedAt: record.CreatedAt,
	}
}

func NewBuildResponse(record build.Build) BuildResponse {
	artifacts := make([]ArtifactResponse, len(record.Artifacts))
	for index, stored := range record.Artifacts {
		artifacts[index] = NewArtifactResponse(stored)
	}
	return BuildResponse{
		ID: record.ID, Runtime: record.Runtime, EntryPoint: record.EntryPoint,
		SourceSHA256: record.SourceSHA256, Artifacts: artifacts,
		CreatedAt: record.CreatedAt,
	}
}

// NewProducedResponse serves a build on its own, so it says where it came from.
func NewProducedResponse(record build.Build, deploymentID, appID string) BuildResponse {
	response := NewBuildResponse(record)
	response.DeploymentID = deploymentID
	response.AppID = appID
	return response
}
