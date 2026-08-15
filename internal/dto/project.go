package dto

import (
	"time"

	"github.com/neurun-io/neurun/internal/domain/project"
)

type CreateProjectRequest struct {
	Name string `json:"name"`
}

type UpdateProjectRequest struct {
	Name *string `json:"name"`
}

type ProjectResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewProjectResponse(record project.Project) ProjectResponse {
	return ProjectResponse{
		ID: record.ID, Name: record.Name,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func NewProjectResponses(records []project.Project) []ProjectResponse {
	responses := make([]ProjectResponse, len(records))
	for index, record := range records {
		responses[index] = NewProjectResponse(record)
	}
	return responses
}
