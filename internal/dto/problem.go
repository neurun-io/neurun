package dto

// Problem is the error body every failing endpoint returns. Code is stable and
// machine-readable; Message is for a person reading a log.
type Problem struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type ErrorResponse struct {
	Error     Problem `json:"error"`
	RequestID string  `json:"request_id,omitempty"`
}
