package deployment

import (
	"encoding/json"
	"time"
)

const snapshotSchema = 1

type deploymentEnvelope struct {
	Schema int                `json:"schema"`
	Record deploymentSnapshot `json:"record"`
}

type projectEnvelope struct {
	Schema int             `json:"schema"`
	Record projectSnapshot `json:"record"`
}

type projectSnapshot struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type runEnvelope struct {
	Schema int         `json:"schema"`
	Record runSnapshot `json:"record"`
}

type ArtifactSnapshot struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Name       string    `json:"name"`
	MediaType  string    `json:"media_type"`
	SizeBytes  int64     `json:"size_bytes"`
	SHA256     string    `json:"sha256"`
	StorageKey string    `json:"storage_key"`
	CreatedAt  time.Time `json:"created_at"`
}

type buildSnapshot struct {
	ID           string             `json:"id"`
	ProjectID    string             `json:"project_id"`
	DeploymentID string             `json:"deployment_id"`
	Number       int                `json:"number"`
	Status       Status             `json:"status"`
	Runtime      Runtime            `json:"runtime"`
	EntryPoint   string             `json:"entrypoint"`
	SourceSHA256 string             `json:"source_sha256"`
	Artifacts    []ArtifactSnapshot `json:"artifacts"`
	Failure      *Failure           `json:"failure,omitempty"`
	StartedAt    time.Time          `json:"started_at"`
	FinishedAt   *time.Time         `json:"finished_at,omitempty"`
}

type deploymentSnapshot struct {
	ID         string           `json:"id"`
	ProjectID  string           `json:"project_id"`
	AppID      string           `json:"app_id"`
	Runtime    Runtime          `json:"runtime"`
	EntryPoint string           `json:"entrypoint"`
	Status     Status           `json:"status"`
	Source     ArtifactSnapshot `json:"source"`
	Builds     []buildSnapshot  `json:"builds"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

type runSnapshot struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"project_id"`
	DeploymentID string          `json:"deployment_id"`
	BuildID      string          `json:"build_id"`
	Status       RunStatus       `json:"status"`
	Input        json.RawMessage `json:"input"`
	Output       json.RawMessage `json:"output,omitempty"`
	Failure      *Failure        `json:"failure,omitempty"`
	Logs         string          `json:"logs,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	FinishedAt   *time.Time      `json:"finished_at,omitempty"`
	RerunOfRunID string          `json:"rerun_of_execution_id,omitempty"`
}

func deploymentToSnapshot(record Deployment) deploymentEnvelope {
	builds := make([]buildSnapshot, len(record.Builds))
	for index, build := range record.Builds {
		artifacts := make([]ArtifactSnapshot, len(build.Artifacts))
		for artifactIndex, stored := range build.Artifacts {
			artifacts[artifactIndex] = stored.Snapshot()
		}
		builds[index] = buildSnapshot{
			ID: build.ID, ProjectID: build.ProjectID,
			DeploymentID: build.DeploymentID,
			Number:       build.Number, Status: build.Status,
			Runtime: build.Runtime, EntryPoint: build.EntryPoint,
			SourceSHA256: build.SourceSHA256, Artifacts: artifacts,
			Failure: CloneFailure(build.Failure), StartedAt: build.StartedAt,
			FinishedAt: cloneTime(build.FinishedAt),
		}
	}
	return deploymentEnvelope{
		Schema: snapshotSchema,
		Record: deploymentSnapshot{
			ID: record.ID, ProjectID: record.ProjectID, AppID: record.AppID,
			Runtime:    record.Runtime,
			EntryPoint: record.EntryPoint, Status: record.Status,
			Source: record.Source.Snapshot(), Builds: builds,
			CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		},
	}
}

func deploymentFromSnapshot(snapshot deploymentSnapshot) Deployment {
	builds := make([]Build, len(snapshot.Builds))
	for index, build := range snapshot.Builds {
		artifacts := make([]Artifact, len(build.Artifacts))
		for artifactIndex, stored := range build.Artifacts {
			artifacts[artifactIndex] = ArtifactFromSnapshot(stored)
		}
		builds[index] = Build{
			ID: build.ID, ProjectID: build.ProjectID,
			DeploymentID: build.DeploymentID,
			Number:       build.Number, Status: build.Status,
			Runtime: build.Runtime, EntryPoint: build.EntryPoint,
			SourceSHA256: build.SourceSHA256, Artifacts: artifacts,
			Failure: CloneFailure(build.Failure), StartedAt: build.StartedAt,
			FinishedAt: cloneTime(build.FinishedAt),
		}
	}
	return Deployment{
		ID: snapshot.ID, ProjectID: snapshot.ProjectID, AppID: snapshot.AppID,
		Runtime:    snapshot.Runtime,
		EntryPoint: snapshot.EntryPoint, Status: snapshot.Status,
		Source: ArtifactFromSnapshot(snapshot.Source), Builds: builds,
		CreatedAt: snapshot.CreatedAt, UpdatedAt: snapshot.UpdatedAt,
	}
}

func (record Artifact) Snapshot() ArtifactSnapshot {
	return ArtifactSnapshot{
		ID: record.ID, Kind: record.Kind, Name: record.Name,
		MediaType: record.MediaType, SizeBytes: record.SizeBytes,
		SHA256: record.SHA256, StorageKey: record.storageKey,
		CreatedAt: record.CreatedAt,
	}
}

func ArtifactFromSnapshot(snapshot ArtifactSnapshot) Artifact {
	return newArtifact(
		snapshot.ID,
		snapshot.Kind,
		snapshot.Name,
		snapshot.MediaType,
		snapshot.SizeBytes,
		snapshot.SHA256,
		snapshot.StorageKey,
		snapshot.CreatedAt,
	)
}

func runToSnapshot(record Run) runEnvelope {
	return runEnvelope{
		Schema: snapshotSchema,
		Record: runSnapshot{
			ID: record.ID, ProjectID: record.ProjectID,
			DeploymentID: record.DeploymentID, BuildID: record.BuildID,
			Status:  record.Status,
			Input:   append(json.RawMessage(nil), record.Input...),
			Output:  append(json.RawMessage(nil), record.Output...),
			Failure: CloneFailure(record.Failure), CreatedAt: record.CreatedAt,
			Logs:         record.Logs,
			StartedAt:    cloneTime(record.StartedAt),
			FinishedAt:   cloneTime(record.FinishedAt),
			RerunOfRunID: record.RerunOfRunID,
		},
	}
}

func runFromSnapshot(snapshot runSnapshot) Run {
	return Run{
		ID: snapshot.ID, ProjectID: snapshot.ProjectID,
		DeploymentID: snapshot.DeploymentID, BuildID: snapshot.BuildID,
		Status:  snapshot.Status,
		Input:   append(json.RawMessage(nil), snapshot.Input...),
		Output:  append(json.RawMessage(nil), snapshot.Output...),
		Failure: CloneFailure(snapshot.Failure), CreatedAt: snapshot.CreatedAt,
		Logs:         snapshot.Logs,
		StartedAt:    cloneTime(snapshot.StartedAt),
		FinishedAt:   cloneTime(snapshot.FinishedAt),
		RerunOfRunID: snapshot.RerunOfRunID,
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func projectToSnapshot(record Project) projectEnvelope {
	return projectEnvelope{
		Schema: snapshotSchema,
		Record: projectSnapshot{
			ID: record.ID, Name: record.Name,
			CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		},
	}
}

func projectFromSnapshot(snapshot projectSnapshot) Project {
	return Project{
		ID: snapshot.ID, Name: snapshot.Name,
		CreatedAt: snapshot.CreatedAt, UpdatedAt: snapshot.UpdatedAt,
	}
}
