package dto

import (
	"time"

	"github.com/neurun-io/neurun/internal/domain/github"
)

// InstallRequest is the installation GitHub redirects back with after somebody
// installs the app on their account.
type InstallRequest struct {
	InstallationID string `json:"installation_id"`
	AccountLogin   string `json:"account_login"`
}

type ConnectRepositoryRequest struct {
	Repository    string `json:"repository"`
	ProductionRef string `json:"production_ref"`
}

type DeployRefRequest struct {
	AppID string `json:"app_id"`
	Ref   string `json:"ref"`
}

type InstallationResponse struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	InstallationID int64     `json:"installation_id"`
	AccountLogin   string    `json:"account_login"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func NewInstallationResponse(record github.Installation) InstallationResponse {
	return InstallationResponse{
		ID: record.ID, OrganizationID: record.OrganizationID,
		InstallationID: record.InstallationID, AccountLogin: record.AccountLogin,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}
