package browsergrpc

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/neurun-io/neurun/internal/browserpb"
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
	tokens     *service.ExecutionTokenService
	supervisor *Supervisor
}

func NewServer(
	sessions *service.BrowserSessionService,
	tokens *service.ExecutionTokenService,
	supervisor *Supervisor,
) (*Server, error) {
	if sessions == nil || tokens == nil || supervisor == nil {
		return nil, errors.New("browser grpc server requires its dependencies")
	}
	return &Server{sessions: sessions, tokens: tokens, supervisor: supervisor}, nil
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
	kind, err := browser.ParseKind(request.GetBrowser())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	opened, err := client.Open(ctx, &browserpb.OpenRequest{
		Browser:          string(kind),
		BrowserProfileId: request.GetBrowserProfileId(),
		AppId:            identity.AppID,
		ExecutionId:      identity.ExecutionID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "browser service: %v", err)
	}

	// Registered against the id the service minted, so the dashboard and the
	// service agree on what to call it.
	record, err := server.sessions.Adopt(ctx, identity.OrganizationID, service.AdoptRequest{
		SessionID:      opened.GetSessionId(),
		AppID:          identity.AppID,
		ExecutionID:    identity.ExecutionID,
		ProfileID:      request.GetBrowserProfileId(),
		Browser:        kind,
		// Every session has a framebuffer, and it is reached through the browser
		// service itself — it multiplexes them all behind one port.
		DisplayAddress: server.supervisor.Address(),
	})
	if err != nil {
		// The browser is already up; leaving it would strand a process nothing
		// can reach.
		_, _ = client.Close(ctx, &browserpb.CloseRequest{SessionId: opened.GetSessionId()})
		return nil, status.Errorf(codes.Internal, "register session: %v", err)
	}
	return sessionMessage(record), nil
}

func (server *Server) Execute(
	ctx context.Context,
	request *browserpb.ExecuteRequest,
) (*browserpb.ExecuteResponse, error) {
	identity, err := server.identify(ctx)
	if err != nil {
		return nil, err
	}
	// Scoped before relaying: a session id from another organization reads as
	// absent rather than as forbidden. The lookup doubles as the lease refresh —
	// a session being driven is a session that is alive, so activity keeps it
	// listed instead of a timer expiring one mid-run.
	if _, err := server.sessions.Touch(
		ctx, identity.OrganizationID, request.GetSessionId(),
	); err != nil {
		return nil, status.Error(codes.NotFound, "browser session not found")
	}
	client, err := server.supervisor.Client(ctx)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	result, err := client.Run(ctx, &browserpb.RunRequest{
		SessionId: request.GetSessionId(),
		Command:   request.GetCommand(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "browser service: %v", err)
	}
	return &browserpb.ExecuteResponse{Result: result.GetResult()}, nil
}

func (server *Server) CloseSession(
	ctx context.Context,
	request *browserpb.CloseSessionRequest,
) (*browserpb.CloseSessionResponse, error) {
	identity, err := server.identify(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := server.sessions.Get(
		ctx, identity.OrganizationID, request.GetSessionId(),
	); err != nil {
		return nil, status.Error(codes.NotFound, "browser session not found")
	}
	client, err := server.supervisor.Client(ctx)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	if _, err := client.Close(
		ctx, &browserpb.CloseRequest{SessionId: request.GetSessionId()},
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
