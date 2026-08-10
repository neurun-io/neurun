package session

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// Password hashing is bcrypt, from golang.org/x/crypto.
//
// The cost is encoded in every hash, so raising it later does not invalidate
// existing ones: an old hash keeps verifying at the cost it was written with.
const (
	// MinimumPasswordLength is counted in runes — a six-character passphrase
	// should not be rejected because its characters are multi-byte.
	MinimumPasswordLength = 6

	// MaximumPasswordBytes is bcrypt's own input limit, not a policy choice.
	// x/crypto refuses a longer password rather than silently truncating it, so
	// anything above this could never be verified. Counted in bytes, so a
	// multi-byte passphrase reaches it sooner than its character count suggests.
	MaximumPasswordBytes = 72
)

var (
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinimumPasswordLength)
	ErrPasswordTooLong  = fmt.Errorf("password must be at most %d bytes", MaximumPasswordBytes)
	ErrPasswordNotUTF8  = errors.New("password must be valid UTF-8")
	ErrHashMalformed    = errors.New("password hash is malformed")
)

// ValidatePassword enforces the length limits before any hashing work happens.
func ValidatePassword(password string) error {
	if !utf8.ValidString(password) {
		return ErrPasswordNotUTF8
	}
	if utf8.RuneCountInString(password) < MinimumPasswordLength {
		return ErrPasswordTooShort
	}
	if len(password) > MaximumPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword returns the bcrypt hash of password at the default cost.
func HashPassword(password string) (string, error) {
	return hashPasswordWithCost(password, bcrypt.DefaultCost)
}

// hashPasswordWithCost exists so tests can hash at the minimum cost instead of
// spending the default cost's ~60ms on every fixture.
func hashPasswordWithCost(password string, cost int) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword reports whether password matches the encoded hash.
//
// A malformed hash returns an error rather than false, so a bad configuration
// value cannot be mistaken for a wrong password. An over-long password is not a
// malformed hash: it simply cannot be the stored one, so it reports no match.
func VerifyPassword(encoded, password string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword),
		errors.Is(err, bcrypt.ErrPasswordTooLong):
		return false, nil
	default:
		return false, fmt.Errorf("%w: %v", ErrHashMalformed, err)
	}
}
