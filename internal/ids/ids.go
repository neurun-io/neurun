package ids

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var (
	base32 = big.NewInt(32)
)

func New(prefix string) (string, error) {
	if err := validatePrefix(prefix); err != nil {
		return "", err
	}

	var raw [16]byte
	millis := uint64(time.Now().UTC().UnixMilli())
	raw[0] = byte(millis >> 40)
	raw[1] = byte(millis >> 32)
	raw[2] = byte(millis >> 24)
	raw[3] = byte(millis >> 16)
	raw[4] = byte(millis >> 8)
	raw[5] = byte(millis)
	if _, err := rand.Read(raw[6:]); err != nil {
		return "", err
	}

	value := new(big.Int).SetBytes(raw[:])
	encoded := make([]byte, 26)
	remainder := new(big.Int)
	for index := len(encoded) - 1; index >= 0; index-- {
		value.QuoRem(value, base32, remainder)
		encoded[index] = crockford[remainder.Int64()]
	}
	return prefix + "_" + string(encoded), nil
}

func Trace() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	ensureNonZero(raw[:])
	return hex.EncodeToString(raw[:]), nil
}

func Span() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	ensureNonZero(raw[:])
	return hex.EncodeToString(raw[:]), nil
}

func ensureNonZero(raw []byte) {
	for _, value := range raw {
		if value != 0 {
			return
		}
	}
	if len(raw) != 0 {
		raw[len(raw)-1] = 1
	}
}

func validatePrefix(prefix string) error {
	if prefix == "" || len(prefix) > 8 {
		return errors.New("ID prefix must contain 1 to 8 lowercase letters")
	}
	for _, character := range prefix {
		if character < 'a' || character > 'z' {
			return errors.New("ID prefix must contain only lowercase ASCII letters")
		}
	}
	return nil
}

func Prefix(value string) string {
	prefix, _, ok := strings.Cut(value, "_")
	if !ok {
		return ""
	}
	return prefix
}

// Validate reports whether value is usable as a record identifier.
//
// Identifiers reach object storage keys and filesystem paths, so the character
// set is deliberately narrower than the ID generator's own output.
func Validate(field, value string) error {
	if value == "" || len(value) > 255 || !utf8.ValidString(value) ||
		value != strings.TrimSpace(value) || value == "." || value == ".." {
		return fmt.Errorf("%s is invalid", field)
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return fmt.Errorf("%s contains an unsafe character", field)
	}
	if windowsDeviceName(value) {
		return fmt.Errorf("%s uses a reserved device name", field)
	}
	return nil
}

// windowsDeviceName guards the legacy DOS device names, which resolve to a
// device rather than a file on Windows regardless of the directory or suffix.
func windowsDeviceName(value string) bool {
	base := value
	if index := strings.IndexByte(base, '.'); index >= 0 {
		base = base[:index]
	}
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}
