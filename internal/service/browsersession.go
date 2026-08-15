package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/neurun-io/neurun/internal/browserservicepb"
	"github.com/neurun-io/neurun/internal/domain/browser"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository/memory"
)

// DisplayStream is the framebuffer pipe, narrowed to what a bridge needs. The
// transport is gRPC; the API layer does not need to know that.
type DisplayStream interface {
	Send(*browserservicepb.DisplayChunk) error
	Recv() (*browserservicepb.DisplayChunk, error)
}

// sessionIDMetadata names the session a display stream belongs to. It is a
// property of the stream, not of a frame, so it travels once in metadata rather
// than on every chunk.
const sessionIDMetadata = "neurun-session-id"

// BrowserSessionService owns the live sessions an organization has open.
type BrowserSessionService struct {
	sessions *memory.BrowserSessionRepository
	now      func() time.Time
	newID    func(string) (string, error)
}

func NewBrowserSessionService(
	sessions *memory.BrowserSessionRepository,
	now func() time.Time,
	newID func(string) (string, error),
) (*BrowserSessionService, error) {
	if sessions == nil {
		return nil, errors.New("browser session service requires its repository")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if newID == nil {
		newID = ids.New
	}
	return &BrowserSessionService{sessions: sessions, now: now, newID: newID}, nil
}

// OpenRequest is what a handler tells the control plane when it starts a
// browser. The organization is never in it: that comes from the credential.
type OpenRequest struct {
	AppID          string
	ExecutionID    string
	ProfileID      string
	Browser        browser.Browser
	DisplayAddress string
}

// Open registers a session and returns it.
func (service *BrowserSessionService) Open(
	ctx context.Context,
	organizationID string,
	request OpenRequest,
) (browser.Session, error) {
	id, err := service.newID("bsn")
	if err != nil {
		return browser.Session{}, err
	}
	now := service.now().UTC().Round(0)
	record, err := browser.NewSession(id, organizationID, request.AppID, request.Browser, now)
	if err != nil {
		return browser.Session{}, err
	}
	record.ExecutionID = strings.TrimSpace(request.ExecutionID)
	record.ProfileID = strings.TrimSpace(request.ProfileID)
	if err := setDisplay(&record, request.DisplayAddress); err != nil {
		return browser.Session{}, err
	}
	if err := service.sessions.Save(ctx, record); err != nil {
		return browser.Session{}, err
	}
	return record, nil
}

// AdoptRequest registers a session the browser service already minted, so both
// sides call it by the same name.
type AdoptRequest struct {
	SessionID      string
	AppID          string
	ExecutionID    string
	ProfileID      string
	Browser        browser.Browser
	DisplayAddress string
}

// Adopt records a session that already exists. The id is the service's, not
// ours: minting a second one here would give the dashboard a name the thing
// holding the browser has never heard of.
func (service *BrowserSessionService) Adopt(
	ctx context.Context,
	organizationID string,
	request AdoptRequest,
) (browser.Session, error) {
	now := service.now().UTC().Round(0)
	record, err := browser.NewSession(
		strings.TrimSpace(request.SessionID), organizationID,
		request.AppID, request.Browser, now,
	)
	if err != nil {
		return browser.Session{}, err
	}
	record.Status = browser.SessionLive
	record.ExecutionID = strings.TrimSpace(request.ExecutionID)
	record.ProfileID = strings.TrimSpace(request.ProfileID)
	if err := setDisplay(&record, request.DisplayAddress); err != nil {
		return browser.Session{}, err
	}
	if err := service.sessions.Save(ctx, record); err != nil {
		return browser.Session{}, err
	}
	return record, nil
}

// Touch renews the lease on a session that is being used. Driving a browser is
// proof it is alive, so the lease follows activity rather than a separate
// heartbeat the SDK would have to remember to send.
func (service *BrowserSessionService) Touch(
	ctx context.Context,
	organizationID string,
	sessionID string,
) (browser.Session, error) {
	record, err := service.sessions.Get(ctx, organizationID, sessionID)
	if err != nil {
		return browser.Session{}, err
	}
	record.UpdatedAt = service.now().UTC().Round(0)
	if err := service.sessions.Save(ctx, record); err != nil {
		return browser.Session{}, err
	}
	return record, nil
}

// Heartbeat refreshes a session's lease and its status. Without it the session
// expires, which is what makes a crashed worker disappear from the list.
func (service *BrowserSessionService) Heartbeat(
	ctx context.Context,
	organizationID string,
	sessionID string,
	status browser.SessionStatus,
) (browser.Session, error) {
	record, err := service.sessions.Get(ctx, organizationID, sessionID)
	if err != nil {
		return browser.Session{}, err
	}
	if status.Valid() {
		record.Status = status
	}
	record.UpdatedAt = service.now().UTC().Round(0)
	if err := service.sessions.Save(ctx, record); err != nil {
		return browser.Session{}, err
	}
	return record, nil
}

func (service *BrowserSessionService) Get(
	ctx context.Context,
	organizationID string,
	sessionID string,
) (browser.Session, error) {
	return service.sessions.Get(ctx, organizationID, sessionID)
}

func (service *BrowserSessionService) List(
	ctx context.Context,
	organizationID string,
) ([]browser.Session, error) {
	return service.sessions.List(ctx, organizationID)
}

// Close forgets the session. The browser is the handler's to stop; this only
// stops the dashboard claiming it is still there.
func (service *BrowserSessionService) Close(
	ctx context.Context,
	organizationID string,
	sessionID string,
) error {
	if _, err := service.sessions.Get(ctx, organizationID, sessionID); err != nil {
		return err
	}
	return service.sessions.Delete(ctx, organizationID, sessionID)
}

// OpenDisplay opens the framebuffer pipe to the session's browser service.
//
// The connection is the caller's to release. It is dialled before anything is
// upgraded so an unreachable service is still an error a client can be told
// about, rather than a socket that opens and immediately dies.
func (service *BrowserSessionService) OpenDisplay(
	ctx context.Context,
	record browser.Session,
) (DisplayStream, func(), error) {
	// Insecure is correct precisely because this never leaves the host. It stops
	// being correct the moment the browser service is on another one.
	connection, err := grpc.NewClient(
		record.DisplayAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("dial browser service: %w", err)
	}
	stream, err := browserservicepb.NewBrowserServiceClient(connection).StreamDisplay(
		metadata.AppendToOutgoingContext(ctx, sessionIDMetadata, record.ID),
	)
	if err != nil {
		connection.Close()
		return nil, nil, fmt.Errorf("open display stream: %w", err)
	}
	return stream, func() { connection.Close() }, nil
}

// setDisplay accepts only a loopback address.
//
// The address is the browser service's gRPC endpoint, and the control plane
// dials it on a viewer's behalf â€” so an address pointing anywhere else would
// turn the display endpoint into a request forger against whatever the server
// can reach.
func setDisplay(record *browser.Session, address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: display must be host:port", browser.ErrInvalid)
	}
	if port == "" {
		return fmt.Errorf("%w: display needs a port", browser.ErrInvalid)
	}
	parsed := net.ParseIP(host)
	if host != "localhost" && (parsed == nil || !parsed.IsLoopback()) {
		return fmt.Errorf(
			"%w: display address must be on loopback", browser.ErrInvalid,
		)
	}
	record.DisplayAddress = address
	return nil
}
