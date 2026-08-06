package artifact

import (
	"crypto/sha256"
	"errors"
	"testing"
)

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
