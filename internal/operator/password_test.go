package operator

import (
	"errors"
	"strings"
	"testing"
)

const testPassword = "correct horse battery staple"

func TestHashPasswordRoundTrip(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, hashAlgorithm+"$") {
		t.Fatalf("hash %q does not declare its algorithm", hash)
	}
	// The plaintext must not survive anywhere in the encoded form.
	if strings.Contains(hash, testPassword) {
		t.Fatal("encoded hash contains the plaintext password")
	}

	matched, err := VerifyPassword(hash, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("correct password did not verify")
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []string{
		"incorrect horse battery staple",
		testPassword + " ",
		" " + testPassword,
		strings.ToUpper(testPassword),
		"",
	} {
		matched, err := VerifyPassword(hash, wrong)
		if err != nil {
			t.Fatalf("verify %q: %v", wrong, err)
		}
		if matched {
			t.Fatalf("password %q unexpectedly verified", wrong)
		}
	}
}

func TestHashPasswordUsesAFreshSalt(t *testing.T) {
	t.Parallel()

	first, err := HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	// Equal hashes for one password would mean a shared or missing salt, which
	// makes the whole set precomputable.
	if first == second {
		t.Fatal("hashing the same password twice produced identical output")
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	if err := ValidatePassword(strings.Repeat("a", MinimumPasswordLength)); err != nil {
		t.Fatalf("minimum-length password rejected: %v", err)
	}
	if err := ValidatePassword(strings.Repeat("a", MinimumPasswordLength-1)); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("short password error = %v, want ErrPasswordTooShort", err)
	}
	if err := ValidatePassword(strings.Repeat("a", MaximumPasswordLength+1)); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("long password error = %v, want ErrPasswordTooLong", err)
	}
	// Length is counted in runes, so a short multi-byte passphrase is not
	// rejected for its byte count.
	if err := ValidatePassword(strings.Repeat("é", MinimumPasswordLength)); err != nil {
		t.Fatalf("multi-byte password of valid rune length rejected: %v", err)
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	t.Parallel()

	for name, hash := range map[string]string{
		"empty":              "",
		"missing fields":     "pbkdf2-sha256$650000",
		"unknown algorithm":  "scrypt$650000$c2FsdA$a2V5",
		"zero iterations":    "pbkdf2-sha256$0$c2FsdA$a2V5",
		"negative iteration": "pbkdf2-sha256$-1$c2FsdA$a2V5",
		"bad base64 salt":    "pbkdf2-sha256$650000$!!!$a2V5",
		"empty key":          "pbkdf2-sha256$650000$c2FsdA$",
	} {
		matched, err := VerifyPassword(hash, testPassword)
		if err == nil {
			t.Errorf("%s: expected an error for hash %q", name, hash)
		}
		// A malformed hash must never read as a successful match.
		if matched {
			t.Errorf("%s: malformed hash %q reported a match", name, hash)
		}
		if err := ValidateHash(hash); err == nil {
			t.Errorf("%s: ValidateHash accepted %q", name, hash)
		}
	}
}

func TestValidateHashAcceptsGeneratedHash(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHash(hash); err != nil {
		t.Fatalf("ValidateHash rejected a generated hash: %v", err)
	}
}

func TestVerifyPasswordAcceptsALowerCostHash(t *testing.T) {
	t.Parallel()

	// The encoded form carries its own iteration count, so raising the default
	// must not invalidate hashes written at an older cost.
	hash, err := hashPasswordWithIterations(testPassword, 1_000)
	if err != nil {
		t.Fatal(err)
	}
	matched, err := VerifyPassword(hash, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("hash written at a lower iteration count did not verify")
	}
}
