package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/neurun-io/neurun/internal/browserservicepb"
	"github.com/neurun-io/neurun/internal/domain/browser"
)

// BrowserProcess is the browser service, narrowed to what driving one needs.
//
// The supervisor implements it. Nothing here knows a process is spawned, which
// host it is on, or that it listens anywhere — it asks for a client and gets
// one.
type BrowserProcess interface {
	Client(ctx context.Context) (browserservicepb.BrowserServiceClient, error)
	Address() string
}

// BrowserDriverService opens browsers and hands back the client to drive them.
//
// It is the one place that knows a browser service exists. Both doors onto it
// come through here — a handler's execution token and an operator's API key —
// so a session opened by either is registered the same way, leased the same
// way, and shows up in the same list. The doors differ in who they let in, and
// in nothing else.
type BrowserDriverService struct {
	sessions *BrowserSessionService
	profiles *BrowserService
	browsers BrowserProcess
}

func NewBrowserDriverService(
	sessions *BrowserSessionService,
	profiles *BrowserService,
	browsers BrowserProcess,
) (*BrowserDriverService, error) {
	if sessions == nil || profiles == nil || browsers == nil {
		return nil, errors.New("browser driver service requires its dependencies")
	}
	return &BrowserDriverService{
		sessions: sessions, profiles: profiles, browsers: browsers,
	}, nil
}

// OpenBrowserRequest is what either door asks for when it wants a browser.
type OpenBrowserRequest struct {
	// chrome or safari. Read only when no profile is named: a profile's identity
	// already says which browser it is, and two answers that could disagree is
	// one answer too many.
	Browser     string
	ProfileID   string
	LoadStorage bool
	// Set when an execution asked. An operator's session has neither, and that
	// is the whole difference between what the two doors open.
	AppID       string
	ExecutionID string
}

// Open spawns a browser, seeds it with what the profile remembers, and
// registers the session the service minted.
//
// Every failure after the browser is up closes it again. A browser nothing has
// a handle to is a process that runs until the host is restarted.
func (service *BrowserDriverService) Open(
	ctx context.Context,
	organizationID string,
	request OpenBrowserRequest,
) (browser.Session, error) {
	if request.LoadStorage && strings.TrimSpace(request.ProfileID) == "" {
		return browser.Session{}, fmt.Errorf(
			"%w: load_storage needs a browser profile: a profile is where a "+
				"session's cookies are kept, and a browser without one keeps none",
			browser.ErrInvalid,
		)
	}
	client, err := service.browsers.Client(ctx)
	if err != nil {
		return browser.Session{}, fmt.Errorf("%w: %s", browser.ErrUnavailable, err)
	}
	// The browser service has no database, so the persona is resolved here and
	// travels with the request.
	kind, persona, jar, err := service.persona(
		ctx, organizationID, request.ProfileID, request.Browser,
	)
	if err != nil {
		return browser.Session{}, err
	}
	if !request.LoadStorage {
		jar = nil
	}

	opened, err := client.Open(ctx, &browserservicepb.OpenRequest{
		Browser:          string(kind),
		BrowserProfileId: request.ProfileID,
		AppId:            request.AppID,
		ExecutionId:      request.ExecutionID,
		Identity:         persona,
	})
	if err != nil {
		return browser.Session{}, fmt.Errorf("%w: %s", browser.ErrUnavailable, err)
	}

	// Before the caller is handed the session, so its first navigation is
	// already carrying the profile's logins.
	if len(jar) > 0 {
		if _, err := client.SetCookies(ctx, &browserservicepb.SetCookiesRequest{
			SessionId: opened.GetSessionId(),
			Cookies:   ServiceCookies(jar),
		}); err != nil {
			// A session that did not get the state it was asked to start from is
			// not the session the caller wanted, and leaving it would strand a
			// browser holding someone else's idea of logged in.
			service.abandon(ctx, client, opened.GetSessionId())
			return browser.Session{}, fmt.Errorf("%w: load cookies: %s", browser.ErrUnavailable, err)
		}
	}

	// Registered against the id the service minted, so the dashboard and the
	// service agree on what to call it.
	record, err := service.sessions.Adopt(ctx, organizationID, AdoptRequest{
		SessionID:   opened.GetSessionId(),
		AppID:       request.AppID,
		ExecutionID: request.ExecutionID,
		ProfileID:   request.ProfileID,
		Browser:     kind,
		// Every session has a framebuffer, and it is reached through the browser
		// service itself — it multiplexes them all behind one port.
		DisplayAddress: service.browsers.Address(),
	})
	if err != nil {
		service.abandon(ctx, client, opened.GetSessionId())
		return browser.Session{}, fmt.Errorf("register session: %w", err)
	}
	return record, nil
}

// Driving authorizes a command against a session and returns the client to
// relay it with.
//
// Scoped before relaying: a session id from another organization reads as
// absent rather than as forbidden. The lookup doubles as the lease refresh — a
// session being driven is a session that is alive, so activity keeps it listed
// instead of a timer expiring one mid-run.
func (service *BrowserDriverService) Driving(
	ctx context.Context,
	organizationID string,
	sessionID string,
) (browserservicepb.BrowserServiceClient, error) {
	if _, err := service.sessions.Touch(ctx, organizationID, sessionID); err != nil {
		return nil, browser.ErrSessionNotFound
	}
	client, err := service.browsers.Client(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", browser.ErrUnavailable, err)
	}
	return client, nil
}

// Close stops the browser and drops the session, keeping what it collected if
// it was asked to.
//
// The record must not outlive the browser either way, so a service that would
// not answer is logged rather than returned: the alternative leaves the
// dashboard showing a session nothing can reach.
func (service *BrowserDriverService) Close(
	ctx context.Context,
	organizationID string,
	sessionID string,
	saveStorage bool,
) error {
	record, err := service.sessions.Get(ctx, organizationID, sessionID)
	if err != nil {
		return browser.ErrSessionNotFound
	}
	if saveStorage && strings.TrimSpace(record.ProfileID) == "" {
		return fmt.Errorf(
			"%w: save_storage needs the session to wear a profile: there is "+
				"nowhere to save what it collected",
			browser.ErrInvalid,
		)
	}
	client, err := service.browsers.Client(ctx)
	if err != nil {
		return fmt.Errorf("%w: %s", browser.ErrUnavailable, err)
	}
	// Read the jar while there is still a browser holding it.
	if saveStorage {
		if err := service.saveCookies(ctx, client, organizationID, record.ProfileID, sessionID); err != nil {
			return err
		}
	}
	if _, err := client.Close(
		ctx, &browserservicepb.CloseRequest{SessionId: sessionID},
	); err != nil {
		slog.Warn("browser service close failed", "session", sessionID, "error", err)
	}
	return service.sessions.Close(ctx, organizationID, sessionID)
}

// abandon stops a browser nothing will be given a handle to. The failure that
// led here is the one worth reporting, so this one is dropped.
func (service *BrowserDriverService) abandon(
	ctx context.Context,
	client browserservicepb.BrowserServiceClient,
	sessionID string,
) {
	_, _ = client.Close(ctx, &browserservicepb.CloseRequest{SessionId: sessionID})
}

// persona resolves what the browser will wear, and which browser that makes it.
//
// A named profile answers both: its identity says chrome or safari, so the
// browser field in the request is not consulted at all — two answers that could
// disagree is one answer too many. Without a profile the caller is saying it
// does not care to keep anything, and gets a coherent machine drawn for this
// session alone.
//
// The profile's jar comes back with it, because the read that answers what to
// wear is the same read that says what it remembers.
func (service *BrowserDriverService) persona(
	ctx context.Context,
	organizationID, profileID, requested string,
) (browser.Browser, []byte, []browser.Cookie, error) {
	identity := browser.Identity{}
	var jar []browser.Cookie

	if strings.TrimSpace(profileID) == "" {
		claimed, err := browser.ParseBrowser(requested)
		if err != nil {
			return "", nil, nil, err
		}
		if identity, err = browser.EphemeralIdentity(claimed); err != nil {
			return "", nil, nil, fmt.Errorf("mint an identity: %w", err)
		}
	} else {
		record, err := service.profiles.Get(ctx, organizationID, profileID)
		if err != nil {
			// A profile from another organization reads as absent, never forbidden.
			return "", nil, nil, browser.ErrNotFound
		}
		identity = record.Identity
		jar = record.Cookies
	}

	document, err := json.Marshal(identity)
	if err != nil {
		return "", nil, nil, fmt.Errorf("encode identity: %w", err)
	}
	return identity.Browser, document, jar, nil
}

// saveCookies reads the session's jar and writes it to the profile.
//
// An empty capture is not written. The browser hands back the whole jar, so a
// write replaces, and a browser that failed to answer looks exactly like one
// holding nothing — writing that would erase a profile's logins on a bad read.
// The cost is that a run which genuinely signed out does not persist the
// signing out, which is the cheaper of the two mistakes.
func (service *BrowserDriverService) saveCookies(
	ctx context.Context,
	client browserservicepb.BrowserServiceClient,
	organizationID, profileID, sessionID string,
) error {
	captured, err := client.GetCookies(
		ctx, &browserservicepb.GetCookiesRequest{SessionId: sessionID},
	)
	if err != nil {
		return fmt.Errorf("%w: capture cookies: %s", browser.ErrUnavailable, err)
	}
	if len(captured.GetCookies()) == 0 {
		slog.Warn("browser session captured no cookies, so the profile is left as it was",
			"session", sessionID, "profile", profileID)
		return nil
	}
	if _, err := service.profiles.SaveCookies(
		ctx, organizationID, profileID, DomainCookies(captured.GetCookies()),
	); err != nil {
		return fmt.Errorf("save cookies: %w", err)
	}
	return nil
}

// ServiceCookies turns a profile's jar into the browser service's shape.
func ServiceCookies(jar []browser.Cookie) []*browserservicepb.Cookie {
	messages := make([]*browserservicepb.Cookie, 0, len(jar))
	for _, cookie := range jar {
		messages = append(messages, &browserservicepb.Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Domain:   cookie.Domain,
			Path:     cookie.Path,
			Expires:  cookie.Expires,
			Secure:   cookie.Secure,
			HttpOnly: cookie.HTTPOnly,
			SameSite: cookie.SameSite,
		})
	}
	return messages
}

// DomainCookies turns the browser service's jar into the profile's own shape.
// Exported because the HTTP layer reads a jar straight out of a session too.
func DomainCookies(messages []*browserservicepb.Cookie) []browser.Cookie {
	jar := make([]browser.Cookie, 0, len(messages))
	for _, message := range messages {
		jar = append(jar, browser.Cookie{
			Name:     message.GetName(),
			Value:    message.GetValue(),
			Domain:   message.GetDomain(),
			Path:     message.GetPath(),
			Expires:  message.Expires,
			Secure:   message.GetSecure(),
			HTTPOnly: message.GetHttpOnly(),
			SameSite: message.GetSameSite(),
		})
	}
	return jar
}
