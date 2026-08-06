package deployment

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNoInstallation = errors.New("no github installation for this organization")
	ErrNotConnected   = errors.New("app is not connected to a repository")
)

// Installation is one organization's GitHub App installation. It holds no
// token: ghinstallation mints a short-lived one per call from the app key.
type Installation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	InstallationID int64     `json:"installation_id"`
	AccountLogin   string    `json:"account_login"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func NewInstallation(
	id, organizationID string,
	installationID int64,
	accountLogin string,
	now time.Time,
) (Installation, error) {
	record := Installation{
		ID:             id,
		OrganizationID: strings.TrimSpace(organizationID),
		InstallationID: installationID,
		AccountLogin:   strings.TrimSpace(accountLogin),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := record.Validate(); err != nil {
		return Installation{}, err
	}
	return record, nil
}

func (record Installation) Validate() error {
	if record.ID == "" || record.OrganizationID == "" {
		return fmt.Errorf("%w: installation requires an id and an organization", ErrInvalid)
	}
	if record.InstallationID <= 0 {
		return fmt.Errorf("%w: installation id must be positive", ErrInvalid)
	}
	if record.AccountLogin == "" {
		return fmt.Errorf("%w: installation requires an account login", ErrInvalid)
	}
	return nil
}
