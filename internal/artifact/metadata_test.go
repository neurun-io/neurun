package artifact

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMetadataIsValidatedNormalizedAndImmutable(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.July, 20, 2, 26, 0, 123, time.FixedZone("WAT", 60*60))
	expiresAt := createdAt.Add(7 * 24 * time.Hour)
	digest := sha256.Sum256([]byte("evidence"))
	metadata, err := NewMetadata(MetadataInput{
		ID:         "art_01TEST",
		ProjectID:  "prj_01TEST",
		Kind:       "http.response_body",
		MediaType:  `Text/Plain; Charset="utf-8"`,
		SizeBytes:  8,
		SHA256:     "sha256:" + strings.ToUpper(hexDigest(digest)),
		StorageKey: "projects/prj_01TEST/artifacts/art_01TEST/body.txt",
		CreatedAt:  createdAt,
		ExpiresAt:  &expiresAt,
	})
	if err != nil {
		t.Fatalf("NewMetadata() error = %v", err)
	}

	if metadata.ID() != "art_01TEST" ||
		metadata.ProjectID() != "prj_01TEST" ||
		metadata.Kind() != "http.response_body" ||
		metadata.MediaType() != "text/plain; charset=utf-8" ||
		metadata.SizeBytes() != 8 ||
		metadata.SHA256() != hexDigest(digest) {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
	if metadata.CreatedAt().Location() != time.UTC {
		t.Fatalf("CreatedAt location = %v, want UTC", metadata.CreatedAt().Location())
	}
	expiry, ok := metadata.ExpiresAt()
	if !ok || !expiry.Equal(expiresAt) || expiry.Location() != time.UTC {
		t.Fatalf("ExpiresAt() = %v, %v", expiry, ok)
	}

	digestCopy := metadata.Digest()
	digestCopy[0] ^= 0xff
	if metadata.SHA256() != hexDigest(digest) {
		t.Fatal("mutating digest copy changed metadata")
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if document["id"] != metadata.ID() ||
		document["project_id"] != metadata.ProjectID() ||
		document["sha256"] != metadata.SHA256() ||
		document["storage_key"] != metadata.StorageKey() {
		t.Fatalf("JSON = %s", encoded)
	}
}

func TestMetadataRejectsIncompleteOrInconsistentRecords(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	digest := sha256.Sum256([]byte("ok"))
	valid := MetadataInput{
		ID:         "art_1",
		ProjectID:  "prj_1",
		Kind:       "screenshot",
		MediaType:  "image/png",
		SizeBytes:  2,
		SHA256:     hexDigest(digest),
		StorageKey: "prj_1/art_1.png",
		CreatedAt:  now,
	}
	before := now.Add(-time.Second)

	tests := []struct {
		name   string
		mutate func(*MetadataInput)
	}{
		{name: "missing id", mutate: func(input *MetadataInput) { input.ID = "" }},
		{name: "id whitespace", mutate: func(input *MetadataInput) { input.ID = "art 1" }},
		{name: "missing project", mutate: func(input *MetadataInput) { input.ProjectID = "" }},
		{name: "uppercase kind", mutate: func(input *MetadataInput) { input.Kind = "Screenshot" }},
		{name: "bad media type", mutate: func(input *MetadataInput) { input.MediaType = "png" }},
		{name: "negative size", mutate: func(input *MetadataInput) { input.SizeBytes = -1 }},
		{name: "short digest", mutate: func(input *MetadataInput) { input.SHA256 = "abcd" }},
		{name: "bad digest", mutate: func(input *MetadataInput) { input.SHA256 = strings.Repeat("z", 64) }},
		{name: "missing key", mutate: func(input *MetadataInput) { input.StorageKey = "" }},
		{name: "zero created", mutate: func(input *MetadataInput) { input.CreatedAt = time.Time{} }},
		{name: "expiry before creation", mutate: func(input *MetadataInput) { input.ExpiresAt = &before }},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := valid
			test.mutate(&input)
			if _, err := NewMetadata(input); err == nil {
				t.Fatal("NewMetadata() unexpectedly succeeded")
			}
		})
	}
}

func TestValidateStorageKeyRejectsUnsafePortablePaths(t *testing.T) {
	t.Parallel()

	valid := []string{"project/artifact.bin", "one", "a/b/c.json"}
	for _, key := range valid {
		if err := ValidateStorageKey(key); err != nil {
			t.Fatalf("ValidateStorageKey(%q) = %v", key, err)
		}
	}

	invalid := []string{
		"",
		"/absolute",
		"../escape",
		"a/../escape",
		"a/./b",
		"a//b",
		"a/b/",
		`a\b`,
		" leading",
		"trailing ",
		"a/\x00/b",
		"a/\n/b",
		string([]byte{'a', '/', 0xff}),
	}
	for _, key := range invalid {
		if err := ValidateStorageKey(key); !errors.Is(err, ErrInvalidStorageKey) {
			t.Fatalf("ValidateStorageKey(%q) = %v, want ErrInvalidStorageKey", key, err)
		}
	}
}

func hexDigest(digest [sha256.Size]byte) string {
	const digits = "0123456789abcdef"
	encoded := make([]byte, len(digest)*2)
	for index, value := range digest {
		encoded[index*2] = digits[value>>4]
		encoded[index*2+1] = digits[value&0x0f]
	}
	return string(encoded)
}
