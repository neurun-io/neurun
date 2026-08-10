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
		Brand:               BrandChrome,
		BrowserVersion:      []uint32{124, 0, 6367, 78},
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

func TestNewRejectsAnUnknownBrowser(t *testing.T) {
	t.Parallel()

	_, err := New("bp_1", "org_1", "shopper", Kind("safari"), nil, time.Now().UTC())
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}

// A profile without an identity is the plain browser: legal, and not the same
// as an incomplete one.
func TestNewAcceptsNoIdentity(t *testing.T) {
	t.Parallel()

	record, err := New("bp_1", "org_1", "plain", KindChrome, nil, time.Now().UTC())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if record.Identity != nil {
		t.Error("identity should stay absent")
	}
}

// The jsonb columns carry array and object CHECKs, and a nil slice marshals to
// null. A fresh profile has to be storable without a session ever running.
func TestNewLeavesStateEmptyRatherThanNil(t *testing.T) {
	t.Parallel()

	record, err := New("bp_1", "org_1", "shopper", KindChrome, validIdentity(), time.Now().UTC())
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
		"no brand":    func(identity *Identity) { identity.Brand = "" },
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
			_, err := New("bp_1", "org_1", "shopper", KindChrome, identity, time.Now().UTC())
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestCaptureReplacesStateAndMovesUpdatedAt(t *testing.T) {
	t.Parallel()

	created := time.Now().UTC()
	record, err := New("bp_1", "org_1", "shopper", KindChrome, validIdentity(), created)
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
	record, err := New("bp_1", "org_1", "shopper", KindChrome, nil, created)
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

func TestSetIdentityStripsPresentationAndKeepsState(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	record, err := New("bp_1", "org_1", "shopper", KindChrome, validIdentity(), now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := record.Capture(
		[]Cookie{{Name: "sid", Domain: ".example.com"}}, nil, nil, now,
	); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if err := record.SetIdentity(nil, now); err != nil {
		t.Fatalf("SetIdentity: %v", err)
	}
	if record.Identity != nil {
		t.Error("identity should be stripped")
	}
	if len(record.Cookies) != 1 {
		t.Errorf("cookies = %+v, want the state left alone", record.Cookies)
	}
}

func TestValidateRejectsANamelessCookie(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	record, err := New("bp_1", "org_1", "shopper", KindChrome, nil, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = record.Capture([]Cookie{{Value: "orphan"}}, nil, nil, now)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}

func TestParseKind(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]Kind{
		"chrome":  KindChrome,
		"Chrome":  KindChrome,
		" chrome": KindChrome,
		"firefox": KindFirefox,
	} {
		got, err := ParseKind(raw)
		if err != nil || got != want {
			t.Errorf("ParseKind(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	if _, err := ParseKind("edge"); !errors.Is(err, ErrInvalid) {
		t.Errorf("ParseKind(edge) error = %v, want ErrInvalid", err)
	}
}
