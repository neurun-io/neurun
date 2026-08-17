package browsergrpc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/neurun-io/neurun/internal/browserpb"
	"github.com/neurun-io/neurun/internal/browserservicepb"
	"github.com/neurun-io/neurun/internal/domain/browser"
	"github.com/neurun-io/neurun/internal/service"
)

// TokenMetadata is where the SDK puts the token the worker gave it.
const TokenMetadata = "neurun-execution-token"

// Server is the SDK's whole view of Neurun's browser support.
//
// It brokers rather than exposes: the SDK asks for a session and drives it by
// id, and never learns that a browser service exists or where it listens. That
// is what makes a session something the dashboard can list and watch, instead of
// something happening on a port only the tenant's code knows about.
type Server struct {
	browserpb.UnimplementedBrowserServer

	sessions   *service.BrowserSessionService
	profiles   *service.BrowserService
	tokens     *service.ExecutionTokenService
	supervisor *Supervisor
}

func NewServer(
	sessions *service.BrowserSessionService,
	profiles *service.BrowserService,
	tokens *service.ExecutionTokenService,
	supervisor *Supervisor,
) (*Server, error) {
	if sessions == nil || profiles == nil || tokens == nil || supervisor == nil {
		return nil, errors.New("browser grpc server requires its dependencies")
	}
	return &Server{
		sessions: sessions, profiles: profiles, tokens: tokens, supervisor: supervisor,
	}, nil
}

// Serve listens on addr, which must be loopback.
//
// The caller on the other end is the tenant's own code. Binding anywhere
// routable would publish an endpoint whose only credential is a token that code
// already holds.
func (server *Server) Serve(ctx context.Context, address string) error {
	if err := requireLoopback(address); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	browserpb.RegisterBrowserServer(grpcServer, server)
	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()
	slog.Info("browser grpc listening", "address", address)
	return grpcServer.Serve(listener)
}

func (server *Server) OpenSession(
	ctx context.Context,
	request *browserpb.OpenSessionRequest,
) (*browserpb.Session, error) {
	identity, err := server.identify(ctx)
	if err != nil {
		return nil, err
	}
	client, err := server.supervisor.Client(ctx)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	if request.GetLoadStorage() && strings.TrimSpace(request.GetBrowserProfileId()) == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"load_storage needs a browser_profile_id: a profile is where a "+
				"session's cookies are kept, and a browser without one keeps none",
		)
	}
	// The browser service has no database, so the persona is resolved here and
	// travels with the request.
	kind, persona, jar, err := server.persona(
		ctx, identity.OrganizationID,
		request.GetBrowserProfileId(), request.GetBrowser(),
	)
	if err != nil {
		return nil, err
	}
	if !request.GetLoadStorage() {
		jar = nil
	}

	opened, err := client.Open(ctx, &browserservicepb.OpenRequest{
		Browser:          string(kind),
		BrowserProfileId: request.GetBrowserProfileId(),
		AppId:            identity.AppID,
		ExecutionId:      identity.ExecutionID,
		Identity:         persona,
	})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "browser service: %v", err)
	}

	// Before the caller is handed the session, so its first navigation is
	// already carrying the profile's logins.
	if len(jar) > 0 {
		if _, err := client.SetCookies(ctx, &browserservicepb.SetCookiesRequest{
			SessionId: opened.GetSessionId(),
			Cookies:   cookieMessages(jar),
		}); err != nil {
			// A session that did not get the state it was asked to start from is
			// not the session the caller wanted, and leaving it would strand a
			// browser holding someone else's idea of logged in.
			_, _ = client.Close(ctx, &browserservicepb.CloseRequest{SessionId: opened.GetSessionId()})
			return nil, status.Errorf(codes.Unavailable, "load cookies: %v", err)
		}
	}

	// Registered against the id the service minted, so the dashboard and the
	// service agree on what to call it.
	record, err := server.sessions.Adopt(ctx, identity.OrganizationID, service.AdoptRequest{
		SessionID:   opened.GetSessionId(),
		AppID:       identity.AppID,
		ExecutionID: identity.ExecutionID,
		ProfileID:   request.GetBrowserProfileId(),
		Browser:     kind,
		// Every session has a framebuffer, and it is reached through the browser
		// service itself — it multiplexes them all behind one port.
		DisplayAddress: server.supervisor.Address(),
	})
	if err != nil {
		// The browser is already up; leaving it would strand a process nothing
		// can reach.
		_, _ = client.Close(ctx, &browserservicepb.CloseRequest{SessionId: opened.GetSessionId()})
		return nil, status.Errorf(codes.Internal, "register session: %v", err)
	}
	return sessionMessage(record), nil
}

func (server *Server) Navigate(
	ctx context.Context,
	request *browserpb.NavigateRequest,
) (*browserpb.NavigateResponse, error) {
	client, err := server.driving(ctx, request.GetSessionId())
	if err != nil {
		return nil, err
	}
	_, err = client.Navigate(ctx, &browserservicepb.NavigateRequest{
		SessionId: request.GetSessionId(),
		Url:       request.GetUrl(),
		Referer:   request.Referer,
	})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "browser service: %v", err)
	}
	return &browserpb.NavigateResponse{}, nil
}

func (server *Server) WaitForNavigation(
	ctx context.Context,
	request *browserpb.WaitForNavigationRequest,
) (*browserpb.WaitForNavigationResponse, error) {
	client, err := server.driving(ctx, request.GetSessionId())
	if err != nil {
		return nil, err
	}
	_, err = client.WaitForNavigation(ctx, &browserservicepb.WaitForNavigationRequest{
		SessionId: request.GetSessionId(),
		// The two enums are the same enum, declared twice, so the number carries.
		WaitUntil: browserservicepb.WaitUntil(request.GetWaitUntil()),
		TimeoutMs: request.GetTimeoutMs(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "browser service: %v", err)
	}
	return &browserpb.WaitForNavigationResponse{}, nil
}

// driving authorizes a command against a session and returns the client to
// relay it with.
//
// Scoped before relaying: a session id from another organization reads as absent
// rather than as forbidden. The lookup doubles as the lease refresh — a session
// being driven is a session that is alive, so activity keeps it listed instead
// of a timer expiring one mid-run.
func (server *Server) driving(
	ctx context.Context,
	sessionID string,
) (browserservicepb.BrowserServiceClient, error) {
	identity, err := server.identify(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := server.sessions.Touch(ctx, identity.OrganizationID, sessionID); err != nil {
		return nil, status.Error(codes.NotFound, "browser session not found")
	}
	client, err := server.supervisor.Client(ctx)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	return client, nil
}

func (server *Server) CloseSession(
	ctx context.Context,
	request *browserpb.CloseSessionRequest,
) (*browserpb.CloseSessionResponse, error) {
	identity, err := server.identify(ctx)
	if err != nil {
		return nil, err
	}
	record, err := server.sessions.Get(ctx, identity.OrganizationID, request.GetSessionId())
	if err != nil {
		return nil, status.Error(codes.NotFound, "browser session not found")
	}
	if request.GetSaveStorage() && strings.TrimSpace(record.ProfileID) == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"save_storage needs the session to wear a profile: there is nowhere "+
				"to save what it collected",
		)
	}
	client, err := server.supervisor.Client(ctx)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	// Read the jar while there is still a browser holding it.
	if request.GetSaveStorage() {
		if err := server.saveCookies(
			ctx, client, identity.OrganizationID, record.ProfileID, request.GetSessionId(),
		); err != nil {
			return nil, err
		}
	}
	if _, err := client.Close(
		ctx, &browserservicepb.CloseRequest{SessionId: request.GetSessionId()},
	); err != nil {
		// The browser may already be gone; the record must not outlive it either
		// way, so this is logged rather than returned.
		slog.Warn("browser service close failed",
			"session", request.GetSessionId(), "error", err)
	}
	if err := server.sessions.Close(
		ctx, identity.OrganizationID, request.GetSessionId(),
	); err != nil {
		return nil, status.Errorf(codes.Internal, "close session: %v", err)
	}
	return &browserpb.CloseSessionResponse{}, nil
}

// persona resolves what the browser will wear, and which browser that makes it.
//
// A named profile answers both: its identity says chrome or safari, so the
// browser field in the request is not consulted at all — two answers that could
// disagree is one answer too many. Without a profile the caller is saying it
// does not care to keep anything, and gets a coherent machine drawn for this
// session alone.
// The profile's jar comes back with it, because the read that answers what to
// wear is the same read that says what it remembers.
func (server *Server) persona(
	ctx context.Context,
	organizationID, profileID, requested string,
) (browser.Browser, []byte, []browser.Cookie, error) {
	identity := browser.Identity{}
	var jar []browser.Cookie

	if strings.TrimSpace(profileID) == "" {
		claimed, err := browser.ParseBrowser(requested)
		if err != nil {
			return "", nil, nil, status.Error(codes.InvalidArgument, err.Error())
		}
		if identity, err = browser.EphemeralIdentity(claimed); err != nil {
			return "", nil, nil, status.Errorf(codes.Internal, "mint an identity: %v", err)
		}
	} else {
		record, err := server.profiles.Get(ctx, organizationID, profileID)
		if err != nil {
			// A profile from another organization reads as absent, never forbidden.
			return "", nil, nil, status.Error(codes.NotFound, "browser profile not found")
		}
		identity = record.Identity
		jar = record.Cookies
	}

	document, err := json.Marshal(identity)
	if err != nil {
		return "", nil, nil, status.Errorf(codes.Internal, "encode identity: %v", err)
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
func (server *Server) saveCookies(
	ctx context.Context,
	client browserservicepb.BrowserServiceClient,
	organizationID, profileID, sessionID string,
) error {
	captured, err := client.GetCookies(
		ctx, &browserservicepb.GetCookiesRequest{SessionId: sessionID},
	)
	if err != nil {
		return status.Errorf(codes.Unavailable, "capture cookies: %v", err)
	}
	if len(captured.GetCookies()) == 0 {
		slog.Warn("browser session captured no cookies, so the profile is left as it was",
			"session", sessionID, "profile", profileID)
		return nil
	}
	if _, err := server.profiles.SaveCookies(
		ctx, organizationID, profileID, domainCookies(captured.GetCookies()),
	); err != nil {
		return status.Errorf(codes.Internal, "save cookies: %v", err)
	}
	return nil
}

func cookieMessages(jar []browser.Cookie) []*browserservicepb.Cookie {
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

func domainCookies(messages []*browserservicepb.Cookie) []browser.Cookie {
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

// identify turns the token into who the caller is. Nothing else in the request
// says, and nothing else is trusted to.
func (server *Server) identify(ctx context.Context) (service.ExecutionIdentity, error) {
	incoming, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return service.ExecutionIdentity{}, status.Error(
			codes.Unauthenticated, "an execution token is required",
		)
	}
	values := incoming.Get(TokenMetadata)
	if len(values) == 0 {
		return service.ExecutionIdentity{}, status.Error(
			codes.Unauthenticated, "an execution token is required",
		)
	}
	identity, err := server.tokens.Resolve(ctx, values[0])
	if err != nil {
		return service.ExecutionIdentity{}, status.Error(
			codes.Unauthenticated, "the execution token was rejected",
		)
	}
	return identity, nil
}

func sessionMessage(record browser.Session) *browserpb.Session {
	return &browserpb.Session{
		Id:               record.ID,
		AppId:            record.AppID,
		ExecutionId:      record.ExecutionID,
		BrowserProfileId: record.ProfileID,
		Browser:          string(record.Browser),
		Status:           string(record.Status),
		StartedAt:        record.StartedAt.UnixMilli(),
	}
}

func requireLoopback(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("browser grpc address must be host:port")
	}
	if host == "localhost" {
		return nil
	}
	parsed := net.ParseIP(strings.TrimSpace(host))
	if parsed == nil || !parsed.IsLoopback() {
		return errors.New("browser grpc address must be on loopback")
	}
	return nil
}
