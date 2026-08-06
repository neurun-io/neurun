package artifact

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrInvalidStorageKey = errors.New("invalid artifact storage key")

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
