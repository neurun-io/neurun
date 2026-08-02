package deployment

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrProjectConflict = errors.New("project conflict")
)

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewProject(id, name string, now time.Time) (Project, error) {
	normalized, err := normalizeProjectName(name)
	if err != nil {
		return Project{}, err
	}
	record := Project{ID: id, Name: normalized, CreatedAt: now, UpdatedAt: now}
	if err := record.Validate(); err != nil {
		return Project{}, err
	}
	return record, nil
}

func (record *Project) Rename(name string, now time.Time) error {
	normalized, err := normalizeProjectName(name)
	if err != nil {
		return err
	}
	record.Name = normalized
	record.UpdatedAt = notBefore(now, record.CreatedAt)
	return record.Validate()
}

func (record Project) Validate() error {
	if err := ValidateIdentifier("project_id", record.ID); err != nil {
		return err
	}
	name, err := normalizeProjectName(record.Name)
	if err != nil || name != record.Name {
		return fmt.Errorf("%w: project name is not normalized", ErrInvalid)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.UpdatedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: project timestamps are invalid", ErrInvalid)
	}
	return nil
}

func normalizeProjectName(raw string) (string, error) {
	return normalizeDisplayName("project", raw)
}

// normalizeDisplayName trims a human-facing name and rejects anything that
// would not survive a round trip through JSON or a terminal.
func normalizeDisplayName(kind, raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 120 || !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: %s name must contain 1 to 120 bytes", ErrInvalid, kind)
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("%w: %s name contains a control character", ErrInvalid, kind)
		}
	}
	return name, nil
}

func notBefore(value, floor time.Time) time.Time {
	if value.Before(floor) {
		return floor
	}
	return value
}
