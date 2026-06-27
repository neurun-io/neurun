package pkg

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"time"
)

func NewBuildID() string {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return time.Now().UTC().Format("20060102T150405Z")
	}
	return time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(random)
}

func NewUUID() string {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return NewBuildID()
	}

	random[6] = (random[6] & 0x0f) | 0x40
	random[8] = (random[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(random)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func SafeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "app"
	}

	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	cleaned := strings.Trim(b.String(), "-.")
	if cleaned == "" {
		return "app"
	}
	return cleaned
}

func tail(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[len(value)-max:]
}

func EnvOr(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
