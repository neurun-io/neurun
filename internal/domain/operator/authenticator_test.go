package operator

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func testAuthenticator(t *testing.T, accounts ...Account) (*Authenticator, *ConfigStore) {
	t.Helper()

	store, err := NewConfigStore(accounts...)
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := NewAuthenticator(store, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return authenticator, store
}

func adminAccount(t *testing.T, username string) Account {
	t.Helper()

	// The minimum cost keeps the suite fast; cost handling is exercised
	// separately in the password tests.
	hash, err := hashPasswordWithCost(testPassword, bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return Account{
		Username:     username,
		Role:         RoleAdmin,
		ProjectID:    "prj_test",
		PasswordHash: hash,
	}
}

func TestLoginIssuesASession(t *testing.T) {
	t.Parallel()

	authenticator, store := testAuthenticator(t, adminAccount(t, "alice"))

	session, token, err := authenticator.Login(context.Background(), "alice", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("login returned an empty token")
	}
	if session.Username != "alice" || session.Role != RoleAdmin {
		t.Fatalf("session = %+v, want alice/admin", session)
	}
	if session.ProjectID != "prj_test" {
		t.Fatalf("session project = %q, want prj_test", session.ProjectID)
	}
	if store.SessionCount() != 1 {
		t.Fatalf("session count = %d, want 1", store.SessionCount())
	}

	resolved, err := authenticator.Session(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != session.ID {
		t.Fatalf("resolved session %q, want %q", resolved.ID, session.ID)
	}
}

func TestLoginIsCaseInsensitiveOnUsername(t *testing.T) {
	t.Parallel()

	authenticator, _ := testAuthenticator(t, adminAccount(t, "alice"))

	if _, _, err := authenticator.Login(context.Background(), "ALICE", testPassword); err != nil {
		t.Fatalf("uppercase username rejected: %v", err)
	}
	if _, _, err := authenticator.Login(context.Background(), "  Alice  ", testPassword); err != nil {
		t.Fatalf("padded username rejected: %v", err)
	}
}

func TestLoginRejectsBadCredentialsIndistinguishably(t *testing.T) {
	t.Parallel()

	disabled := adminAccount(t, "carol")
	disabled.Disabled = true
	authenticator, _ := testAuthenticator(t, adminAccount(t, "alice"), disabled)

	cases := map[string]struct{ username, password string }{
		"wrong password":   {"alice", "not the password"},
		"unknown user":     {"nobody", testPassword},
		"disabled account": {"carol", testPassword},
	}
	for name, credentials := range cases {
		_, _, err := authenticator.Login(context.Background(), credentials.username, credentials.password)
		// All three must be the same error: the response is what a caller sees,
		// and it must not distinguish these cases.
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: error = %v, want ErrInvalidCredentials", name, err)
		}
	}
}

func TestRepeatedFailuresDoNotLockTheAccountOut(t *testing.T) {
	t.Parallel()

	authenticator, _ := testAuthenticator(t, adminAccount(t, "alice"))

	// Sign-in throttling was removed: rate limiting belongs at the edge, not in
	// the authenticator, and an in-process counter never covered a multi-replica
	// deployment anyway. Repeated failures must therefore stay plain credential
	// errors, and the correct password must still work immediately afterwards.
	for range 10 {
		if _, _, err := authenticator.Login(context.Background(), "alice", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("error = %v, want ErrInvalidCredentials", err)
		}
	}
	if _, _, err := authenticator.Login(context.Background(), "alice", testPassword); err != nil {
		t.Fatalf("login after repeated failures failed: %v", err)
	}
}

func TestSessionExpires(t *testing.T) {
	t.Parallel()

	authenticator, store := testAuthenticator(t, adminAccount(t, "alice"))
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	authenticator.Now = func() time.Time { return now }

	_, token, err := authenticator.Login(context.Background(), "alice", testPassword)
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(authenticator.SessionTTL + time.Second)
	if _, err := authenticator.Session(context.Background(), token); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("error = %v, want ErrSessionExpired", err)
	}
	// The expired session is dropped on read, so it cannot be probed repeatedly.
	if store.SessionCount() != 0 {
		t.Fatalf("session count = %d, want 0 after expiry", store.SessionCount())
	}
}

func TestLogoutRevokesTheSession(t *testing.T) {
	t.Parallel()

	authenticator, store := testAuthenticator(t, adminAccount(t, "alice"))

	_, token, err := authenticator.Login(context.Background(), "alice", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := authenticator.Logout(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.Session(context.Background(), token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
	if store.SessionCount() != 0 {
		t.Fatalf("session count = %d, want 0", store.SessionCount())
	}
	// Logging out twice is not an error.
	if err := authenticator.Logout(context.Background(), token); err != nil {
		t.Fatalf("second logout failed: %v", err)
	}
}

func TestSessionRejectsAnUnknownToken(t *testing.T) {
	t.Parallel()

	authenticator, _ := testAuthenticator(t, adminAccount(t, "alice"))

	for _, token := range []string{"", "not-a-real-token"} {
		if _, err := authenticator.Session(context.Background(), token); !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("token %q: error = %v, want ErrSessionNotFound", token, err)
		}
	}
}

func TestPruneSessionsRemovesOnlyExpiredSessions(t *testing.T) {
	t.Parallel()

	authenticator, store := testAuthenticator(t, adminAccount(t, "alice"), adminAccount(t, "bob"))
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	authenticator.Now = func() time.Time { return now }

	if _, _, err := authenticator.Login(context.Background(), "alice", testPassword); err != nil {
		t.Fatal(err)
	}
	now = now.Add(authenticator.SessionTTL - time.Minute)
	if _, _, err := authenticator.Login(context.Background(), "bob", testPassword); err != nil {
		t.Fatal(err)
	}

	// Advance past alice's expiry but not bob's.
	now = now.Add(2 * time.Minute)
	removed, err := authenticator.PruneSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if store.SessionCount() != 1 {
		t.Fatalf("session count = %d, want 1", store.SessionCount())
	}
}

func TestRoleScopes(t *testing.T) {
	t.Parallel()

	if got := RoleAdmin.Scopes(); len(got) != 1 || got[0] != "*" {
		t.Fatalf("admin scopes = %v, want [*]", got)
	}

	// A viewer must not be able to start or stop execution.
	viewer := RoleViewer.Scopes()
	for _, forbidden := range []string{"*", "functions:invoke", "jobs:write"} {
		for _, scope := range viewer {
			if scope == forbidden {
				t.Errorf("viewer unexpectedly granted %q", forbidden)
			}
		}
	}
	if len(viewer) == 0 {
		t.Fatal("viewer granted no scopes at all")
	}

	if Role("root").Valid() {
		t.Fatal("unknown role reported as valid")
	}
	if _, err := ParseRole("root"); err == nil {
		t.Fatal("ParseRole accepted an unknown role")
	}
	if role, err := ParseRole("  ADMIN "); err != nil || role != RoleAdmin {
		t.Fatalf("ParseRole(\"  ADMIN \") = %q, %v", role, err)
	}
}

func TestNewMemoryStoreRejectsBadAccounts(t *testing.T) {
	t.Parallel()

	valid := adminAccount(t, "alice")

	duplicate := valid
	duplicate.Username = "ALICE"
	if _, err := NewConfigStore(valid, duplicate); err == nil {
		t.Fatal("duplicate username differing only in case was accepted")
	}

	badRole := valid
	badRole.Role = Role("root")
	if _, err := NewConfigStore(badRole); err == nil {
		t.Fatal("invalid role was accepted")
	}

	badHash := valid
	badHash.PasswordHash = "not-a-hash"
	if _, err := NewConfigStore(badHash); err == nil {
		t.Fatal("malformed password hash was accepted")
	}

	empty := valid
	empty.Username = "   "
	if _, err := NewConfigStore(empty); err == nil {
		t.Fatal("empty username was accepted")
	}
}

func TestAccountsOmitsPasswordHashes(t *testing.T) {
	t.Parallel()

	_, store := testAuthenticator(t, adminAccount(t, "alice"))

	for _, account := range store.Accounts() {
		if account.PasswordHash != "" {
			t.Fatal("Accounts() leaked a password hash")
		}
	}
}
