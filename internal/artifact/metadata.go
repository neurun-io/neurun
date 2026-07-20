package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidMetadata   = errors.New("invalid artifact metadata")
	ErrInvalidStorageKey = errors.New("invalid artifact storage key")
)

// MetadataInput contains all fields required to construct immutable Metadata.
// CreatedAt must be supplied by the caller so persistence and replay do not
// depend on a hidden wall clock.
type MetadataInput struct {
	ID         string
	ProjectID  string
	Kind       string
	MediaType  string
	SizeBytes  int64
	SHA256     string
	StorageKey string
	CreatedAt  time.Time
	ExpiresAt  *time.Time
}

// Metadata is an immutable artifact record. Its fields are deliberately
// private; accessors return values or copies and no mutating methods exist.
type Metadata struct {
	id         string
	projectID  string
	kind       string
	mediaType  string
	sizeBytes  int64
	digest     [sha256.Size]byte
	storageKey string
	createdAt  time.Time
	expiresAt  time.Time
	hasExpiry  bool
}

// NewMetadata validates and normalizes a complete immutable artifact record.
func NewMetadata(input MetadataInput) (Metadata, error) {
	if err := validateIdentifier("id", input.ID); err != nil {
		return Metadata{}, err
	}
	if err := validateIdentifier("project_id", input.ProjectID); err != nil {
		return Metadata{}, err
	}
	if err := validateKind(input.Kind); err != nil {
		return Metadata{}, err
	}

	mediaType, parameters, err := mime.ParseMediaType(input.MediaType)
	if err != nil || mediaType == "" || !strings.Contains(mediaType, "/") {
		return Metadata{}, fmt.Errorf("%w: malformed media_type", ErrInvalidMetadata)
	}
	// FormatMediaType makes parameter ordering and casing stable in API
	// responses and persistence comparisons.
	mediaType = mime.FormatMediaType(strings.ToLower(mediaType), parameters)
	if mediaType == "" {
		return Metadata{}, fmt.Errorf("%w: malformed media_type", ErrInvalidMetadata)
	}

	if input.SizeBytes < 0 {
		return Metadata{}, fmt.Errorf("%w: size_bytes cannot be negative", ErrInvalidMetadata)
	}
	digestText := strings.ToLower(strings.TrimSpace(input.SHA256))
	digestText = strings.TrimPrefix(digestText, "sha256:")
	decoded, err := hex.DecodeString(digestText)
	if err != nil || len(decoded) != sha256.Size {
		return Metadata{}, fmt.Errorf("%w: sha256 must contain 64 hexadecimal characters", ErrInvalidMetadata)
	}
	if err := ValidateStorageKey(input.StorageKey); err != nil {
		return Metadata{}, err
	}
	if input.CreatedAt.IsZero() {
		return Metadata{}, fmt.Errorf("%w: created_at is required", ErrInvalidMetadata)
	}

	createdAt := input.CreatedAt.UTC().Round(0)
	var expiresAt time.Time
	hasExpiry := input.ExpiresAt != nil
	if hasExpiry {
		if input.ExpiresAt.IsZero() {
			return Metadata{}, fmt.Errorf("%w: expires_at cannot be zero", ErrInvalidMetadata)
		}
		expiresAt = input.ExpiresAt.UTC().Round(0)
		if !expiresAt.After(createdAt) {
			return Metadata{}, fmt.Errorf("%w: expires_at must be after created_at", ErrInvalidMetadata)
		}
	}

	var digest [sha256.Size]byte
	copy(digest[:], decoded)
	return Metadata{
		id:         input.ID,
		projectID:  input.ProjectID,
		kind:       input.Kind,
		mediaType:  mediaType,
		sizeBytes:  input.SizeBytes,
		digest:     digest,
		storageKey: input.StorageKey,
		createdAt:  createdAt,
		expiresAt:  expiresAt,
		hasExpiry:  hasExpiry,
	}, nil
}

func (metadata Metadata) ID() string         { return metadata.id }
func (metadata Metadata) ProjectID() string  { return metadata.projectID }
func (metadata Metadata) Kind() string       { return metadata.kind }
func (metadata Metadata) MediaType() string  { return metadata.mediaType }
func (metadata Metadata) SizeBytes() int64   { return metadata.sizeBytes }
func (metadata Metadata) StorageKey() string { return metadata.storageKey }
func (metadata Metadata) CreatedAt() time.Time {
	return metadata.createdAt
}

// SHA256 returns the lowercase hexadecimal digest without a "sha256:" prefix.
func (metadata Metadata) SHA256() string {
	return hex.EncodeToString(metadata.digest[:])
}

// Digest returns a value copy of the raw SHA-256 digest.
func (metadata Metadata) Digest() [sha256.Size]byte {
	return metadata.digest
}

// ExpiresAt reports the optional retention deadline.
func (metadata Metadata) ExpiresAt() (time.Time, bool) {
	return metadata.expiresAt, metadata.hasExpiry
}

// MarshalJSON exposes the stable public representation without making the Go
// record mutable.
func (metadata Metadata) MarshalJSON() ([]byte, error) {
	var expiresAt *time.Time
	if metadata.hasExpiry {
		value := metadata.expiresAt
		expiresAt = &value
	}
	return json.Marshal(struct {
		ID         string     `json:"id"`
		ProjectID  string     `json:"project_id"`
		Kind       string     `json:"kind"`
		MediaType  string     `json:"media_type"`
		SizeBytes  int64      `json:"size_bytes"`
		SHA256     string     `json:"sha256"`
		StorageKey string     `json:"storage_key"`
		CreatedAt  time.Time  `json:"created_at"`
		ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	}{
		ID:         metadata.ID(),
		ProjectID:  metadata.ProjectID(),
		Kind:       metadata.Kind(),
		MediaType:  metadata.MediaType(),
		SizeBytes:  metadata.SizeBytes(),
		SHA256:     metadata.SHA256(),
		StorageKey: metadata.StorageKey(),
		CreatedAt:  metadata.CreatedAt(),
		ExpiresAt:  expiresAt,
	})
}

// ValidateStorageKey requires a portable, relative, canonical object key.
func ValidateStorageKey(storageKey string) error {
	if storageKey == "" || len(storageKey) > 1024 || !utf8.ValidString(storageKey) {
		return fmt.Errorf("%w: key must be between 1 and 1024 bytes", ErrInvalidStorageKey)
	}
	if storageKey != strings.TrimSpace(storageKey) ||
		strings.HasPrefix(storageKey, "/") ||
		strings.HasSuffix(storageKey, "/") ||
		strings.ContainsAny(storageKey, "\\\x00") {
		return fmt.Errorf("%w: key must be a clean relative slash path", ErrInvalidStorageKey)
	}
	for _, character := range storageKey {
		if unicode.IsControl(character) {
			return fmt.Errorf("%w: key contains a control character", ErrInvalidStorageKey)
		}
	}
	for _, component := range strings.Split(storageKey, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("%w: key contains an unsafe path component", ErrInvalidStorageKey)
		}
	}
	if path.Clean(storageKey) != storageKey {
		return fmt.Errorf("%w: key is not canonical", ErrInvalidStorageKey)
	}
	return nil
}

func validateIdentifier(field, value string) error {
	if value == "" || len(value) > 255 || value != strings.TrimSpace(value) {
		return fmt.Errorf("%w: %s must be a non-empty canonical identifier", ErrInvalidMetadata, field)
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return fmt.Errorf("%w: %s contains whitespace or a control character", ErrInvalidMetadata, field)
		}
	}
	return nil
}

func validateKind(kind string) error {
	if err := validateIdentifier("kind", kind); err != nil {
		return err
	}
	for _, character := range kind {
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '.' ||
			character == '_' ||
			character == '-' {
			continue
		}
		return fmt.Errorf("%w: kind must use lowercase token characters", ErrInvalidMetadata)
	}
	return nil
}
