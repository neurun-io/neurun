package operator

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Password hashing uses PBKDF2-HMAC-SHA256 from the standard library. Argon2id
// would resist GPU attack better, but it is not in the standard library and this
// module has no third-party dependencies; keeping it that way is worth more here
// than the margin, provided the iteration count stays current.
//
// Encoded form, one self-describing string so the cost can be raised later
// without invalidating existing hashes:
//
//	pbkdf2-sha256$<iterations>$<base64 salt>$<base64 key>
const (
	hashAlgorithm = "pbkdf2-sha256"

	// OWASP's 2023 floor for PBKDF2-HMAC-SHA256 is 600,000 iterations.
	DefaultIterations = 650_000

	saltBytes = 16
	keyBytes  = 32

	// Long enough to be worth typing carefully; the ceiling only exists so a
	// megabyte of input cannot be turned into CPU time.
	MinimumPasswordLength = 12
	MaximumPasswordLength = 256
)

var (
	ErrPasswordTooShort   = fmt.Errorf("password must be at least %d characters", MinimumPasswordLength)
	ErrPasswordTooLong    = fmt.Errorf("password must be at most %d characters", MaximumPasswordLength)
	ErrPasswordNotUTF8    = errors.New("password must be valid UTF-8")
	ErrHashMalformed      = errors.New("password hash is malformed")
	ErrHashUnsupportedAlg = errors.New("password hash algorithm is not supported")
)

// ValidatePassword enforces length limits before any hashing work happens.
func ValidatePassword(password string) error {
	if !utf8.ValidString(password) {
		return ErrPasswordNotUTF8
	}
	// Counted in runes: a 12-character passphrase should not be rejected because
	// its characters are multi-byte.
	length := utf8.RuneCountInString(password)
	if length < MinimumPasswordLength {
		return ErrPasswordTooShort
	}
	if length > MaximumPasswordLength {
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword returns the encoded hash of password using a fresh random salt.
func HashPassword(password string) (string, error) {
	return hashPasswordWithIterations(password, DefaultIterations)
}

func hashPasswordWithIterations(password string, iterations int) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	if iterations <= 0 {
		return "", errors.New("iterations must be positive")
	}

	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, keyBytes)
	if err != nil {
		return "", fmt.Errorf("derive password key: %w", err)
	}
	return encodeHash(iterations, salt, key), nil
}

func encodeHash(iterations int, salt, key []byte) string {
	encoding := base64.RawStdEncoding
	return strings.Join([]string{
		hashAlgorithm,
		strconv.Itoa(iterations),
		encoding.EncodeToString(salt),
		encoding.EncodeToString(key),
	}, "$")
}

type parsedHash struct {
	iterations int
	salt       []byte
	key        []byte
}

func parseHash(encoded string) (parsedHash, error) {
	fields := strings.Split(encoded, "$")
	if len(fields) != 4 {
		return parsedHash{}, ErrHashMalformed
	}
	if fields[0] != hashAlgorithm {
		return parsedHash{}, ErrHashUnsupportedAlg
	}
	iterations, err := strconv.Atoi(fields[1])
	if err != nil || iterations <= 0 {
		return parsedHash{}, ErrHashMalformed
	}
	encoding := base64.RawStdEncoding
	salt, err := encoding.DecodeString(fields[2])
	if err != nil || len(salt) == 0 {
		return parsedHash{}, ErrHashMalformed
	}
	key, err := encoding.DecodeString(fields[3])
	if err != nil || len(key) == 0 {
		return parsedHash{}, ErrHashMalformed
	}
	return parsedHash{iterations: iterations, salt: salt, key: key}, nil
}

// ValidateHash reports whether encoded is a hash this build can verify. Used at
// startup so a malformed operator account fails fast rather than at first login.
func ValidateHash(encoded string) error {
	_, err := parseHash(encoded)
	return err
}

// VerifyPassword reports whether password matches the encoded hash.
//
// The comparison is constant-time. A malformed hash returns false with an error
// rather than panicking, so a bad configuration value cannot be mistaken for a
// wrong password.
func VerifyPassword(encoded, password string) (bool, error) {
	parsed, err := parseHash(encoded)
	if err != nil {
		return false, err
	}
	candidate, err := pbkdf2.Key(sha256.New, password, parsed.salt, parsed.iterations, len(parsed.key))
	if err != nil {
		return false, fmt.Errorf("derive password key: %w", err)
	}
	return subtle.ConstantTimeCompare(candidate, parsed.key) == 1, nil
}
