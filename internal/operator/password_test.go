package operator

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const testPassword = "correct horse battery staple"

func TestHashPasswordRoundTrip(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	// Every bcrypt hash declares its variant and cost in this prefix.
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("hash %q is not a bcrypt hash", hash)
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

	hash, err := hashPasswordWithCost(testPassword, bcrypt.MinCost)
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

	first, err := hashPasswordWithCost(testPassword, bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	second, err := hashPasswordWithCost(testPassword, bcrypt.MinCost)
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
	if err := ValidatePassword(strings.Repeat("a", MaximumPasswordBytes+1)); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("long password error = %v, want ErrPasswordTooLong", err)
	}
	// The minimum is counted in runes, so a short multi-byte passphrase is not
	// rejected for its byte count.
	if err := ValidatePassword(strings.Repeat("é", MinimumPasswordLength)); err != nil {
		t.Fatalf("multi-byte password of valid rune length rejected: %v", err)
	}
	// The maximum is counted in bytes, because that is the limit bcrypt itself
	// imposes: 40 two-byte runes is 80 bytes and cannot be hashed.
	if err := ValidatePassword(strings.Repeat("é", 40)); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("multi-byte password over the byte limit error = %v, want ErrPasswordTooLong", err)
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	t.Parallel()

	for name, hash := range map[string]string{
		"empty":            "",
		"not a hash":       "hunter2",
		"truncated":        "$2a$10$",
		"unknown variant":  "$9z$10$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUV",
		"impossible cost":  "$2a$99$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUV",
		"retired pbkdf2":   "pbkdf2-sha256$650000$c2FsdA$a2V5",
		"missing cost":     "$2a$$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUV",
		"plausible length": strings.Repeat("x", 60),
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

func TestVerifyPasswordTreatsAnOverLongPasswordAsNoMatch(t *testing.T) {
	t.Parallel()

	hash, err := hashPasswordWithCost(testPassword, bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	// bcrypt refuses to hash more than 72 bytes. That makes an over-long input
	// impossible to be the stored password, but it is not a malformed hash and
	// must not surface as a configuration fault.
	matched, err := VerifyPassword(hash, strings.Repeat("a", MaximumPasswordBytes+1))
	if err != nil {
		t.Fatalf("over-long password returned %v, want nil", err)
	}
	if matched {
		t.Fatal("over-long password reported a match")
	}
}

func TestValidateHashAcceptsGeneratedHash(t *testing.T) {
	t.Parallel()

	hash, err := hashPasswordWithCost(testPassword, bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHash(hash); err != nil {
		t.Fatalf("ValidateHash rejected a generated hash: %v", err)
	}
}

func TestVerifyPasswordAcceptsALowerCostHash(t *testing.T) {
	t.Parallel()

	// Every hash carries its own cost, so raising the default must not
	// invalidate hashes written at an older one.
	hash, err := hashPasswordWithCost(testPassword, bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatal(err)
	}
	if cost != bcrypt.MinCost {
		t.Fatalf("cost = %d, want %d", cost, bcrypt.MinCost)
	}
	matched, err := VerifyPassword(hash, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("hash written at a lower cost did not verify")
	}
}
