package dto

import (
	"time"

	"github.com/neurun-io/neurun/internal/domain/browser"
)

// BrowserSessionResponse is a live session as an operator sees it. The display
// address is deliberately absent: a viewer asks the control plane to stream and
// never learns where the framebuffer actually is.
type BrowserSessionResponse struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	ExecutionID string    `json:"execution_id,omitempty"`
	ProfileID   string    `json:"browser_profile_id,omitempty"`
	Browser     string    `json:"browser"`
	Status      string    `json:"status"`
	HasDisplay  bool      `json:"has_display"`
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func NewBrowserSessionResponse(record browser.Session) BrowserSessionResponse {
	return BrowserSessionResponse{
		ID: record.ID, AppID: record.AppID, ExecutionID: record.ExecutionID,
		ProfileID: record.ProfileID, Browser: string(record.Browser),
		Status: string(record.Status), HasDisplay: record.HasDisplay(),
		StartedAt: record.StartedAt, UpdatedAt: record.UpdatedAt,
	}
}

func NewBrowserSessionResponses(records []browser.Session) []BrowserSessionResponse {
	responses := make([]BrowserSessionResponse, len(records))
	for index, record := range records {
		responses[index] = NewBrowserSessionResponse(record)
	}
	return responses
}
