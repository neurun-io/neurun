package dto

type RuntimePayload struct {
	Ctx    map[string]any `json:"ctx"`
	Inputs map[string]any `json:"inputs"`
}
