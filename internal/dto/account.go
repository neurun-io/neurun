package dto

import (
	"time"

	"github.com/neurun-io/neurun/internal/domain/account"
)

type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UpdateUserRequest struct {
	Email    *string `json:"email"`
	Disabled *bool   `json:"disabled"`
}

type CreateKeyRequest struct {
	UserID string   `json:"user_id"`
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type KeyResponse struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	UserID         string     `json:"user_id,omitempty"`
	Name           string     `json:"name"`
	Prefix         string     `json:"prefix"`
	Scopes         []string   `json:"scopes"`
	CreatedAt      time.Time  `json:"created_at"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
}

// CreatedKeyResponse carries the one and only sight of a key's secret.
type CreatedKeyResponse struct {
	KeyResponse
	Secret string `json:"secret"`
}

func NewUserResponse(record account.User) UserResponse {
	return UserResponse{
		ID: record.ID, Email: record.Email, Disabled: record.Disabled,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func NewUserResponses(records []account.User) []UserResponse {
	responses := make([]UserResponse, len(records))
	for index, record := range records {
		responses[index] = NewUserResponse(record)
	}
	return responses
}

func NewKeyResponse(record account.Key) KeyResponse {
	return KeyResponse{
		ID: record.ID, OrganizationID: record.OrganizationID, UserID: record.UserID,
		Name: record.Name, Prefix: record.Prefix, Scopes: record.Scopes,
		CreatedAt: record.CreatedAt, RevokedAt: record.RevokedAt,
	}
}

func NewKeyResponses(records []account.Key) []KeyResponse {
	responses := make([]KeyResponse, len(records))
	for index, record := range records {
		responses[index] = NewKeyResponse(record)
	}
	return responses
}

func NewCreatedKeyResponse(record account.CreatedKey) CreatedKeyResponse {
	return CreatedKeyResponse{
		KeyResponse: NewKeyResponse(record.Key),
		Secret:      record.Secret,
	}
}
