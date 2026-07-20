package ids

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"time"
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
