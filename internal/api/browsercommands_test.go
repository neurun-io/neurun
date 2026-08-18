package api

import (
	"net/http"
	"testing"

	"github.com/neurun-io/neurun/internal/browserservicepb"
)

// The three enums are spelled in JSON and numbered on the wire, and every one
// of them defaults to the browser's own default rather than to a value invented
// here.
func TestTheEnumsDefaultToTheBrowsersOwnDefault(t *testing.T) {
	waitUntil, err := parseWaitUntil("")
	if err != nil || waitUntil != browserservicepb.WaitUntil_WAIT_UNTIL_LOAD {
		t.Errorf("an unnamed wait should be load, got %v (%v)", waitUntil, err)
	}
	button, err := parseMouseButton("")
	if err != nil || button != browserservicepb.MouseButton_MOUSE_BUTTON_LEFT {
		t.Errorf("an unnamed button should be left, got %v (%v)", button, err)
	}
	align, err := parseScrollAlign("")
	if err != nil || align != browserservicepb.ScrollAlign_SCROLL_ALIGN_CENTER {
		t.Errorf("an unnamed alignment should be centre, got %v (%v)", align, err)
	}
}

func TestTheEnumsAreCaseAndSpaceInsensitive(t *testing.T) {
	if align, err := parseScrollAlign("  Bottom "); err != nil ||
		align != browserservicepb.ScrollAlign_SCROLL_ALIGN_BOTTOM {
		t.Errorf("expected bottom, got %v (%v)", align, err)
	}
	if button, err := parseMouseButton("RIGHT"); err != nil ||
		button != browserservicepb.MouseButton_MOUSE_BUTTON_RIGHT {
		t.Errorf("expected right, got %v (%v)", button, err)
	}
}

// A value nobody serves is refused rather than silently becoming the default,
// because a caller who wrote "middle" and meant it should not get a left click.
func TestAnUnknownEnumValueIsRefused(t *testing.T) {
	if _, err := parseWaitUntil("settled"); err == nil {
		t.Error("expected an unknown wait to be refused")
	}
	if _, err := parseMouseButton("thumb"); err == nil {
		t.Error("expected an unknown button to be refused")
	}
	if _, err := parseScrollAlign("middle"); err == nil {
		t.Error("expected an unknown alignment to be refused")
	}
}

// The proto enum keeps zero for unspecified and the browser's own starts left
// at zero, so the two differ by one all the way down. This is the test that
// catches someone replacing the switch with a cast.
func TestTheButtonNumbersAreNotTheBrowsersOwn(t *testing.T) {
	button, err := parseMouseButton("left")
	if err != nil {
		t.Fatalf("parse left: %v", err)
	}
	if button == 0 {
		t.Error("zero is unspecified on this side; left is one")
	}
}

// Nothing here reaches a service, so the assertion is that the command routes
// exist at all and are behind the credential rather than in front of it.
func TestBrowserCommandRoutesRejectAnonymousCallers(t *testing.T) {
	server := newTestServer(t)
	sessions := "/v1/browser-sessions/bsn_1"
	for _, route := range []struct {
		method string
		target string
	}{
		{http.MethodPost, "/v1/browser-sessions"},
		{http.MethodPost, sessions + "/navigate"},
		{http.MethodPost, sessions + "/wait-for-navigation"},
		{http.MethodGet, sessions + "/node?selector=input"},
		{http.MethodPost, sessions + "/mouse-move"},
		{http.MethodPost, sessions + "/click"},
		{http.MethodPost, sessions + "/type"},
		{http.MethodPost, sessions + "/scroll"},
		{http.MethodPost, sessions + "/scroll-to"},
		{http.MethodGet, sessions + "/cookies"},
		{http.MethodPut, sessions + "/cookies"},
	} {
		t.Run(route.method+" "+route.target, func(t *testing.T) {
			recorded := do(t, server, route.method, route.target)
			if recorded.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", recorded.Code)
			}
		})
	}
}
