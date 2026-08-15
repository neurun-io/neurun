package dto

import (
	"time"

	appdomain "github.com/neurun-io/neurun/internal/domain/app"
)

// CreateAppRequest carries the repository because an app is only ever created
// from one: there is no other way to get source into a deployment.
type CreateAppRequest struct {
	ProjectID     string `json:"project_id"`
	Name          string `json:"name"`
	Repository    string `json:"repository"`
	ProductionRef string `json:"production_ref"`
}

type UpdateAppRequest struct {
	Name *string `json:"name"`
}

type AppResponse struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	Name          string    `json:"name"`
	Repository    string    `json:"repository,omitempty"`
	ProductionRef string    `json:"production_ref,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func NewAppResponse(record appdomain.App) AppResponse {
	return AppResponse{
		ID: record.ID, ProjectID: record.ProjectID, Name: record.Name,
		Repository: record.Repository, ProductionRef: record.ProductionRef,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func NewAppResponses(records []appdomain.App) []AppResponse {
	responses := make([]AppResponse, len(records))
	for index, record := range records {
		responses[index] = NewAppResponse(record)
	}
	return responses
}
