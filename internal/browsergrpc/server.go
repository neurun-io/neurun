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
//
// What it adds over the driver underneath is who is asking. The execution token
// is the only thing in a request that is trusted, and resolving it is the first
// thing every method does.
type Server struct {
	browserpb.UnimplementedBrowserServer

	driver *service.BrowserDriverService
	tokens *service.ExecutionTokenService
}

func NewServer(
	driver *service.BrowserDriverService,
	tokens *service.ExecutionTokenService,
) (*Server, error) {
	if driver == nil || tokens == nil {
		return nil, errors.New("browser grpc server requires its dependencies")
	}
	return &Server{driver: driver, tokens: tokens}, nil
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
	record, err := server.driver.Open(ctx, identity.OrganizationID, service.OpenBrowserRequest{
		Browser:     request.GetBrowser(),
		ProfileID:   request.GetBrowserProfileId(),
		LoadStorage: request.GetLoadStorage(),
		AppID:       identity.AppID,
		ExecutionID: identity.ExecutionID,
	})
	if err != nil {
		return nil, statusOf(err)
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
		return nil, relayed(err)
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
		return nil, relayed(err)
	}
	return &browserpb.WaitForNavigationResponse{}, nil
}

func (server *Server) GetNode(
	ctx context.Context,
	request *browserpb.GetNodeRequest,
) (*browserpb.GetNodeResponse, error) {
	client, err := server.driving(ctx, request.GetSessionId())
	if err != nil {
		return nil, err
	}
	found, err := client.GetNode(ctx, &browserservicepb.GetNodeRequest{
		SessionId: request.GetSessionId(),
		Selector:  request.GetSelector(),
		TimeoutMs: request.GetTimeoutMs(),
	})
	if err != nil {
		return nil, relayed(err)
	}
	return &browserpb.GetNodeResponse{Node: nodeMessage(found.GetNode())}, nil
}

func (server *Server) HumanMouseMove(
	ctx context.Context,
	request *browserpb.HumanMouseMoveRequest,
) (*browserpb.HumanMouseMoveResponse, error) {
	client, err := server.driving(ctx, request.GetSessionId())
	if err != nil {
		return nil, err
	}
	_, err = client.HumanMouseMove(ctx, &browserservicepb.HumanMouseMoveRequest{
		SessionId: request.GetSessionId(),
		X:         request.X,
		Y:         request.Y,
		Selector:  request.GetSelector(),
	})
	if err != nil {
		return nil, relayed(err)
	}
	return &browserpb.HumanMouseMoveResponse{}, nil
}

func (server *Server) HumanMouseClick(
	ctx context.Context,
	request *browserpb.HumanMouseClickRequest,
) (*browserpb.HumanMouseClickResponse, error) {
	client, err := server.driving(ctx, request.GetSessionId())
	if err != nil {
		return nil, err
	}
	_, err = client.HumanMouseClick(ctx, &browserservicepb.HumanMouseClickRequest{
		SessionId: request.GetSessionId(),
		X:         request.X,
		Y:         request.Y,
		Selector:  request.GetSelector(),
		// The two enums are the same enum, declared twice, so the number carries.
		Button:  browserservicepb.MouseButton(request.GetButton()),
		Count:   request.GetCount(),
		DelayMs: request.GetDelayMs(),
	})
	if err != nil {
		return nil, relayed(err)
	}
	return &browserpb.HumanMouseClickResponse{}, nil
}

func (server *Server) HumanType(
	ctx context.Context,
	request *browserpb.HumanTypeRequest,
) (*browserpb.HumanTypeResponse, error) {
	client, err := server.driving(ctx, request.GetSessionId())
	if err != nil {
		return nil, err
	}
	_, err = client.HumanType(ctx, &browserservicepb.HumanTypeRequest{
		SessionId:  request.GetSessionId(),
		Text:       request.GetText(),
		Selector:   request.GetSelector(),
		DelayMinMs: request.GetDelayMinMs(),
		DelayMaxMs: request.GetDelayMaxMs(),
	})
	if err != nil {
		return nil, relayed(err)
	}
	return &browserpb.HumanTypeResponse{}, nil
}

func (server *Server) HumanScrollY(
	ctx context.Context,
	request *browserpb.HumanScrollYRequest,
) (*browserpb.HumanScrollYResponse, error) {
	client, err := server.driving(ctx, request.GetSessionId())
	if err != nil {
		return nil, err
	}
	_, err = client.HumanScrollY(ctx, &browserservicepb.HumanScrollYRequest{
		SessionId: request.GetSessionId(),
		DeltaY:    request.GetDeltaY(),
	})
	if err != nil {
		return nil, relayed(err)
	}
	return &browserpb.HumanScrollYResponse{}, nil
}

func (server *Server) HumanScrollYTo(
	ctx context.Context,
	request *browserpb.HumanScrollYToRequest,
) (*browserpb.HumanScrollYToResponse, error) {
	client, err := server.driving(ctx, request.GetSessionId())
	if err != nil {
		return nil, err
	}
	_, err = client.HumanScrollYTo(ctx, &browserservicepb.HumanScrollYToRequest{
		SessionId: request.GetSessionId(),
		Selector:  request.GetSelector(),
		Align:     browserservicepb.ScrollAlign(request.GetAlign()),
	})
	if err != nil {
		return nil, relayed(err)
	}
	return &browserpb.HumanScrollYToResponse{}, nil
}

func (server *Server) GetCookies(
	ctx context.Context,
	request *browserpb.GetCookiesRequest,
) (*browserpb.GetCookiesResponse, error) {
	client, err := server.driving(ctx, request.GetSessionId())
	if err != nil {
		return nil, err
	}
	captured, err := client.GetCookies(ctx, &browserservicepb.GetCookiesRequest{
		SessionId: request.GetSessionId(),
	})
	if err != nil {
		return nil, relayed(err)
	}
	return &browserpb.GetCookiesResponse{
		Cookies: publicCookies(captured.GetCookies()),
	}, nil
}

func (server *Server) SetCookies(
	ctx context.Context,
	request *browserpb.SetCookiesRequest,
) (*browserpb.SetCookiesResponse, error) {
	client, err := server.driving(ctx, request.GetSessionId())
	if err != nil {
		return nil, err
	}
	_, err = client.SetCookies(ctx, &browserservicepb.SetCookiesRequest{
		SessionId: request.GetSessionId(),
		Cookies:   serviceCookies(request.GetCookies()),
	})
	if err != nil {
		return nil, relayed(err)
	}
	return &browserpb.SetCookiesResponse{}, nil
}

func (server *Server) CloseSession(
	ctx context.Context,
	request *browserpb.CloseSessionRequest,
) (*browserpb.CloseSessionResponse, error) {
	identity, err := server.identify(ctx)
	if err != nil {
		return nil, err
	}
	if err := server.driver.Close(
		ctx, identity.OrganizationID, request.GetSessionId(), request.GetSaveStorage(),
	); err != nil {
		return nil, statusOf(err)
	}
	return &browserpb.CloseSessionResponse{}, nil
}

// driving authorizes a command against a session and returns the client to
// relay it with. The lookup doubles as the lease refresh.
func (server *Server) driving(
	ctx context.Context,
	sessionID string,
) (browserservicepb.BrowserServiceClient, error) {
	identity, err := server.identify(ctx)
	if err != nil {
		return nil, err
	}
	client, err := server.driver.Driving(ctx, identity.OrganizationID, sessionID)
	if err != nil {
		return nil, statusOf(err)
	}
	return client, nil
}

// relayed turns a browser service failure into one for the caller.
//
// A command the browser refused is the browser's answer, not this hop's, so its
// code and message are kept: a selector that matched nothing has to read as
// NotFound rather than as the service being down. Anything without a status is
// the hop itself failing, and that is Unavailable.
func relayed(err error) error {
	if answered, ok := status.FromError(err); ok {
		return answered.Err()
	}
	return status.Errorf(codes.Unavailable, "browser service: %v", err)
}

// statusOf gives a domain failure its gRPC code.
func statusOf(err error) error {
	switch {
	case errors.Is(err, browser.ErrSessionNotFound), errors.Is(err, browser.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, browser.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, browser.ErrUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// nodeMessage carries an element across the two contracts. Absent stays absent:
// a response with no node is what a selector that matched nothing looks like.
func nodeMessage(found *browserservicepb.Node) *browserpb.Node {
	if found == nil {
		return nil
	}
	attributes := make([]*browserpb.Attribute, 0, len(found.GetAttributes()))
	for _, attribute := range found.GetAttributes() {
		attributes = append(attributes, &browserpb.Attribute{
			Name:  attribute.GetName(),
			Value: attribute.GetValue(),
		})
	}
	return &browserpb.Node{
		NodeId:     found.GetNodeId(),
		LocalName:  found.GetLocalName(),
		NodeType:   found.GetNodeType(),
		Attributes: attributes,
		Text:       found.GetText(),
		Html:       found.GetHtml(),
		X:          found.GetX(),
		Y:          found.GetY(),
		Width:      found.GetWidth(),
		Height:     found.GetHeight(),
	}
}

// publicCookies and serviceCookies are the same jar in the two contracts. They
// exist because the messages are declared twice rather than shared, which is
// what keeps the public contract from moving whenever the internal one does.
func publicCookies(jar []*browserservicepb.Cookie) []*browserpb.Cookie {
	messages := make([]*browserpb.Cookie, 0, len(jar))
	for _, cookie := range jar {
		messages = append(messages, &browserpb.Cookie{
			Name:     cookie.GetName(),
			Value:    cookie.GetValue(),
			Domain:   cookie.GetDomain(),
			Path:     cookie.GetPath(),
			Expires:  cookie.Expires,
			Secure:   cookie.GetSecure(),
			HttpOnly: cookie.GetHttpOnly(),
			SameSite: cookie.GetSameSite(),
		})
	}
	return messages
}

func serviceCookies(jar []*browserpb.Cookie) []*browserservicepb.Cookie {
	messages := make([]*browserservicepb.Cookie, 0, len(jar))
	for _, cookie := range jar {
		messages = append(messages, &browserservicepb.Cookie{
			Name:     cookie.GetName(),
			Value:    cookie.GetValue(),
			Domain:   cookie.GetDomain(),
			Path:     cookie.GetPath(),
			Expires:  cookie.Expires,
			Secure:   cookie.GetSecure(),
			HttpOnly: cookie.GetHttpOnly(),
			SameSite: cookie.GetSameSite(),
		})
	}
	return messages
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
		StartedAt:        record.StartedAt.Unix(),
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
