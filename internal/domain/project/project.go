// Package project owns an organization's grouping of apps. It knows nothing
// about what those apps deploy.
package project

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/neurun-io/neurun/internal/ids"
)

var (
	ErrInvalid  = errors.New("invalid project")
	ErrNotFound = errors.New("project not found")
	ErrConflict = errors.New("project conflict")
)

type Project struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func New(id, organizationID, name string, now time.Time) (Project, error) {
	normalized, err := NormalizeName(name)
	if err != nil {
		return Project{}, err
	}
	record := Project{
		ID:             id,
		OrganizationID: strings.TrimSpace(organizationID),
		Name:           normalized,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := record.Validate(); err != nil {
		return Project{}, err
	}
	return record, nil
}

func (record *Project) Rename(name string, now time.Time) error {
	normalized, err := NormalizeName(name)
	if err != nil {
		return err
	}
	record.Name = normalized
	if now.Before(record.CreatedAt) {
		now = record.CreatedAt
	}
	record.UpdatedAt = now
	return record.Validate()
}

func (record Project) Validate() error {
	if err := ValidateIdentifier("project_id", record.ID); err != nil {
		return err
	}
	if record.OrganizationID == "" {
		return fmt.Errorf("%w: project requires an organization", ErrInvalid)
	}
	name, err := NormalizeName(record.Name)
	if err != nil || name != record.Name {
		return fmt.Errorf("%w: project name is not normalized", ErrInvalid)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.UpdatedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: project timestamps are invalid", ErrInvalid)
	}
	return nil
}

func ValidateIdentifier(field, value string) error {
	if err := ids.Validate(field, value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

// NormalizeName trims a human-facing name and rejects anything that would not
// survive a round trip through JSON or a terminal.
func NormalizeName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 120 || !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: project name must contain 1 to 120 bytes", ErrInvalid)
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("%w: project name contains a control character", ErrInvalid)
		}
	}
	return name, nil
}
