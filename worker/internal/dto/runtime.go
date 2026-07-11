package dto

type RuntimePayload struct {
	Ctx       map[string]any    `json:"ctx"`
	Inputs    map[string]any    `json:"inputs"`
	InputRefs map[string]string `json:"input_refs,omitempty"`
}
