package browser

import (
	"errors"
	"testing"
	"time"
)

func validIdentity() *Identity {
	return &Identity{
		OS:        OSWindows,
		OSVersion: "10.0.0",
		Platform: Platform{
			NavigatorPlatform: PlatformWin32,
			Version:           "10.0.0",
		},
		Browser:        BrowserChrome,
		BrowserVersion: []uint32{124, 0, 6367, 78},
		Screen: Screen{
			LogicalWidth: 1920, LogicalHeight: 1080,
			OriginalWidth: 1920, OriginalHeight: 1080, DensityPixelRatio: 1,
		},
		HardwareConcurrency: 8,
		Memory:              8,
		GPU: GPU{
			Vendor:        "NVIDIA",
			WebGLRenderer: "ANGLE (NVIDIA GeForce RTX 3080)",
			WebGLVendor:   "Google Inc. (NVIDIA)",
		},
		Geo:      "US",
		Language: []string{"en-US", "en"},
	}
}

// No identity means no opinion, not a bare browser. The seed is the profile id,
// so the persona holds still for that profile and differs from the next one's â€”
// a fleet that all looks like one machine is a fleet a detector can name.
func TestNewSeedsAnIdentityWhenNoneIsGiven(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	first, err := New("bp_1", "org_1", "plain", nil, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := first.Identity.Validate(); err != nil {
		t.Fatalf("seeded identity is invalid: %v", err)
	}

	again, err := New("bp_1", "org_1", "plain", nil, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if again.Identity.GPU != first.Identity.GPU ||
		again.Identity.Screen != first.Identity.Screen {
		t.Error("the same profile drew a different machine")
	}

	other, err := New("bp_2", "org_1", "plain", nil, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if other.Identity.GPU == first.Identity.GPU &&
		other.Identity.Screen == first.Identity.Screen {
		t.Error("two profiles drew the same machine")
	}
}

// An ephemeral session gets a persona too, and one the caller can pin to a
// browser without pinning anything else.
func TestEphemeralIdentityHonoursTheBrowser(t *testing.T) {
	t.Parallel()

	identity, err := EphemeralIdentity(BrowserChrome)
	if err != nil {
		t.Fatalf("EphemeralIdentity: %v", err)
	}
	if identity.Browser != BrowserChrome {
		t.Errorf("browser = %q, want chrome", identity.Browser)
	}
	// Chrome is the only browser the catalogue offers, and Linux is the only
	// desktop it ships on, so anything else is the binding coming loose.
	if identity.OS != OSLinux {
		t.Errorf("os = %q, want Linux", identity.OS)
	}
	if err := identity.Validate(); err != nil {
		t.Errorf("ephemeral identity is invalid: %v", err)
	}
}

// The jsonb columns carry array and object CHECKs, and a nil slice marshals to
// null. A fresh profile has to be storable without a session ever running.
func TestNewLeavesStateEmptyRatherThanNil(t *testing.T) {
	t.Parallel()

	record, err := New("bp_1", "org_1", "shopper", validIdentity(), time.Now().UTC())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if record.Cookies == nil || record.LocalStorage == nil || record.SessionStorage == nil {
		t.Fatalf("state is nil: %+v", record)
	}
}

func TestNewRejectsAnIncompleteIdentity(t *testing.T) {
	t.Parallel()

	for name, mutate := range map[string]func(*Identity){
		"no os":       func(identity *Identity) { identity.OS = "" },
		"no browser":  func(identity *Identity) { identity.Browser = "" },
		"bad geo":     func(identity *Identity) { identity.Geo = "ZZ" },
		"no language": func(identity *Identity) { identity.Language = nil },
		"no memory":   func(identity *Identity) { identity.Memory = 0 },
		"no gpu":      func(identity *Identity) { identity.GPU = GPU{} },
		"flat screen": func(identity *Identity) { identity.Screen.DensityPixelRatio = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			identity := validIdentity()
			mutate(identity)
			_, err := New("bp_1", "org_1", "shopper", identity, time.Now().UTC())
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestCaptureReplacesStateAndMovesUpdatedAt(t *testing.T) {
	t.Parallel()

	created := time.Now().UTC()
	record, err := New("bp_1", "org_1", "shopper", validIdentity(), created)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	captured := created.Add(time.Minute)
	err = record.Capture(
		[]Cookie{{Name: "sid", Value: "secret", Domain: ".example.com", Path: "/"}},
		Storage{"https://example.com": {"seen": "1"}},
		nil,
		captured,
	)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if len(record.Cookies) != 1 || record.Cookies[0].Name != "sid" {
		t.Fatalf("cookies = %+v", record.Cookies)
	}
	if !record.UpdatedAt.Equal(captured) {
		t.Errorf("UpdatedAt = %v, want %v", record.UpdatedAt, captured)
	}
	// Capture normalises, so an omitted area is empty rather than null.
	if record.SessionStorage == nil {
		t.Error("session storage should be empty, not nil")
	}
}

// UpdatedAt is compared against CreatedAt by Validate, so a clock that steps
// backwards must not produce a record the repository then refuses to store.
func TestCaptureNeverPredatesCreation(t *testing.T) {
	t.Parallel()

	created := time.Now().UTC()
	record, err := New("bp_1", "org_1", "shopper", nil, created)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := record.Capture(nil, nil, nil, created.Add(-time.Hour)); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if record.UpdatedAt.Before(record.CreatedAt) {
		t.Errorf("UpdatedAt = %v, before CreatedAt %v", record.UpdatedAt, record.CreatedAt)
	}
}

func TestSetIdentityReplacesPresentationAndKeepsState(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	record, err := New("bp_1", "org_1", "shopper", validIdentity(), now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := record.Capture(
		[]Cookie{{Name: "sid", Domain: ".example.com"}}, nil, nil, now,
	); err != nil {
		t.Fatalf("Capture: %v", err)
	}

	replacement := *validIdentity()
	replacement.Memory = 4
	if err := record.SetIdentity(replacement, now); err != nil {
		t.Fatalf("SetIdentity: %v", err)
	}
	if record.Identity.Memory != 4 {
		t.Errorf("memory = %d, want the identity replaced", record.Identity.Memory)
	}
	if len(record.Cookies) != 1 {
		t.Errorf("cookies = %+v, want the state left alone", record.Cookies)
	}
}

func TestValidateRejectsANamelessCookie(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	record, err := New("bp_1", "org_1", "shopper", nil, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = record.Capture([]Cookie{{Value: "orphan"}}, nil, nil, now)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}

func TestParseBrowser(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]Browser{
		"chrome":  BrowserChrome,
		"Chrome":  BrowserChrome,
		" chrome": BrowserChrome,
		"safari":  BrowserSafari,
	} {
		got, err := ParseBrowser(raw)
		if err != nil || got != want {
			t.Errorf("ParseBrowser(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"edge", "firefox", ""} {
		if _, err := ParseBrowser(raw); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseBrowser(%q) error = %v, want ErrInvalid", raw, err)
		}
	}
}
