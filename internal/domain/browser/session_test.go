package browser

import (
	"errors"
	"testing"
	"time"
)

func TestASessionAnExecutionOpenedNamesItsApp(t *testing.T) {
	now := time.Now().UTC()
	record, err := NewSession("bsn_1", "org_1", "app_1", BrowserChrome, now)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	if record.AppID != "app_1" {
		t.Errorf("expected the app to be kept, got %q", record.AppID)
	}
	if record.Status != SessionStarting {
		t.Errorf("a new session starts as starting, got %q", record.Status)
	}
}

// An API key is not a run, so a session opened over the HTTP API has no app and
// no execution. Demanding one would only get a made-up value written down.
func TestASessionAnOperatorOpenedNeedsNoApp(t *testing.T) {
	now := time.Now().UTC()
	record, err := NewSession("bsn_1", "org_1", "", BrowserChrome, now)
	if err != nil {
		t.Fatalf("a session without an app should be valid: %v", err)
	}
	if record.AppID != "" || record.ExecutionID != "" {
		t.Errorf("expected neither an app nor an execution, got %q and %q",
			record.AppID, record.ExecutionID)
	}
}

func TestASessionStillNeedsAnIdentityAndABrowser(t *testing.T) {
	now := time.Now().UTC()
	for name, record := range map[string]Session{
		"no id":           {OrganizationID: "org_1", Browser: BrowserChrome, Status: SessionLive, StartedAt: now, UpdatedAt: now},
		"no organization": {ID: "bsn_1", Browser: BrowserChrome, Status: SessionLive, StartedAt: now, UpdatedAt: now},
		"no browser":      {ID: "bsn_1", OrganizationID: "org_1", Status: SessionLive, StartedAt: now, UpdatedAt: now},
		"no status":       {ID: "bsn_1", OrganizationID: "org_1", Browser: BrowserChrome, StartedAt: now, UpdatedAt: now},
		"no clock":        {ID: "bsn_1", OrganizationID: "org_1", Browser: BrowserChrome, Status: SessionLive},
	} {
		t.Run(name, func(t *testing.T) {
			if err := record.Validate(); !errors.Is(err, ErrInvalid) {
				t.Errorf("expected ErrInvalid, got %v", err)
			}
		})
	}
}
