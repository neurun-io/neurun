package account

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// KeyCredential is the stored half of an API key: enough to verify a presented
// secret and build a principal from it, and nothing that could reconstruct one.
type KeyCredential struct {
	ID             string
	OrganizationID string
	Scopes         []string
	Digest         []byte
}

// SecretPrefix splits the public lookup prefix off a presented key. The form is
// neu_<environment>_<prefix>.<secret>; only the part before the dot is indexed.
func SecretPrefix(raw string) (string, bool) {
	prefix, _, found := strings.Cut(strings.TrimSpace(raw), ".")
	if !found || prefix == "" {
		return "", false
	}
	return prefix, true
}

func HashSecret(raw string) []byte {
	digest := sha256.Sum256([]byte(raw))
	return digest[:]
}

// MintSecret returns a fresh key: its public lookup prefix and the full secret
// the caller will present. The secret is shown once and never stored.
func MintSecret() (string, string, error) {
	prefixRaw, secretRaw := make([]byte, 6), make([]byte, 32)
	if _, err := rand.Read(prefixRaw); err != nil {
		return "", "", fmt.Errorf("generate key prefix: %w", err)
	}
	if _, err := rand.Read(secretRaw); err != nil {
		return "", "", fmt.Errorf("generate key secret: %w", err)
	}
	prefix := "neu_live_" + hex.EncodeToString(prefixRaw)
	return prefix, prefix + "." + hex.EncodeToString(secretRaw), nil
}

// Matches reports whether raw is the secret this credential was issued for.
// The comparison is constant time so a wrong key cannot be refined by timing.
func (credential KeyCredential) Matches(raw string) bool {
	if len(credential.Digest) != sha256.Size {
		return false
	}
	digest := sha256.Sum256([]byte(raw))
	return subtle.ConstantTimeCompare(credential.Digest, digest[:]) == 1
}
