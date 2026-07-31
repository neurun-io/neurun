package buildinfo

var (
	Version       = "0.1.0"
	Commit        = "unknown"
	BuiltAt       = "unknown"
	APIVersion    = "v1"
	SchemaVersion = "1"
)

type Info struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	BuiltAt       string `json:"built_at"`
	APIVersion    string `json:"api_version"`
	SchemaVersion string `json:"schema_version"`
}

func Current() Info {
	return Info{
		Version:       Version,
		Commit:        Commit,
		BuiltAt:       BuiltAt,
		APIVersion:    APIVersion,
		SchemaVersion: SchemaVersion,
	}
}
