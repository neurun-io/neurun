package dto

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/neurun-io/neurun/internal/domain/browser"
)

func profileWithSecrets() browser.Profile {
	now := time.Now().UTC()
	return browser.Profile{
		ID:             "bp_1",
		OrganizationID: "org_1",
		Name:           "shopper",
		Identity: browser.Identity{
			OS:      browser.OSWindows,
			Browser: browser.BrowserChrome,
			Geo:     "US",
			Proxy:   "socks5://user:hunter2@proxy.example.com:1080",
		},
		Cookies: []browser.Cookie{
			{Name: "sid", Value: "s3cr3t-session", Domain: ".example.com", Path: "/"},
		},
		LocalStorage:   browser.Storage{"https://example.com": {"token": "bearer-abc"}},
		SessionStorage: browser.Storage{"https://shop.example.com": {"cart": "1"}},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// The profile response is the one a list view renders. Nothing in it may be
// worth stealing.
func TestBrowserProfileResponseDisclosesNoSecret(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(NewBrowserProfileResponse(profileWithSecrets()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(encoded)

	for _, secret := range []string{"s3cr3t-session", "hunter2", "bearer-abc", "proxy.example.com"} {
		if strings.Contains(body, secret) {
			t.Errorf("response leaks %q: %s", secret, body)
		}
	}
}

func TestBrowserProfileResponseReportsWhatItHidesInstead(t *testing.T) {
	t.Parallel()

	response := NewBrowserProfileResponse(profileWithSecrets())

	if !response.Identity.ProxySet {
		t.Fatal("a set proxy should be reported as set")
	}
	if response.Identity.Proxy != "" {
		t.Errorf("proxy = %q, want empty", response.Identity.Proxy)
	}
	if len(response.Cookies) != 1 {
		t.Fatalf("cookies = %+v", response.Cookies)
	}
	cookie := response.Cookies[0]
	if cookie.Name != "sid" || cookie.Domain != ".example.com" {
		t.Errorf("cookie = %+v, want it still identifiable", cookie)
	}
	if cookie.ValueSize != len("s3cr3t-session") {
		t.Errorf("ValueSize = %d, want %d", cookie.ValueSize, len("s3cr3t-session"))
	}
	want := []string{"https://example.com", "https://shop.example.com"}
	if len(response.StorageOrigins) != len(want) {
		t.Fatalf("StorageOrigins = %v, want %v", response.StorageOrigins, want)
	}
	for index, origin := range want {
		if response.StorageOrigins[index] != origin {
			t.Errorf("StorageOrigins = %v, want %v sorted", response.StorageOrigins, want)
			break
		}
	}
}

// The state endpoint is the one place values are returned, which is why it
// takes a write scope.
func TestBrowserProfileStateResponseReturnsTheValues(t *testing.T) {
	t.Parallel()

	state := NewBrowserProfileStateResponse(profileWithSecrets())
	if len(state.Cookies) != 1 || state.Cookies[0].Value != "s3cr3t-session" {
		t.Fatalf("cookies = %+v, want the value intact", state.Cookies)
	}
	if state.LocalStorage["https://example.com"]["token"] != "bearer-abc" {
		t.Errorf("local storage = %+v", state.LocalStorage)
	}
}

// The browser a profile is comes from the identity, so a list view never has to
// reach into it and the two can never disagree.
func TestBrowserProfileResponseReportsTheIdentitysBrowser(t *testing.T) {
	t.Parallel()

	record := profileWithSecrets()
	record.Identity.Browser = browser.BrowserSafari
	if response := NewBrowserProfileResponse(record); response.Browser != "safari" {
		t.Errorf("Browser = %q, want safari", response.Browser)
	}
}
