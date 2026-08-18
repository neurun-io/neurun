package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/neurun-io/neurun/internal/browserservicepb"
	"github.com/neurun-io/neurun/internal/domain/browser"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/service"
)

// The browser commands, for a caller holding an API key.
//
// They are the same commands the SDK gets and they go through the same driver;
// what differs is the credential at the door. A handler inside an execution
// holds a token that says which app and which run it is, and gets a session
// belonging to that run. An API key holds neither, and gets a session belonging
// to the organization and nothing else.
//
// Every command names its session in the path and carries the rest as JSON, so
// a caller can drive a browser with nothing but an HTTP client.

func (server *Server) openBrowserSession(ctx *gin.Context) {
	if !server.browsersDriveable(ctx) {
		return
	}
	var request dto.OpenBrowserSessionRequest
	if !server.bindJSON(ctx, &request) {
		return
	}
	record, err := server.browserDriver.Open(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		service.OpenBrowserRequest{
			Browser:     request.Browser,
			ProfileID:   request.ProfileID,
			LoadStorage: request.LoadStorage,
		},
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, dto.NewBrowserSessionResponse(record))
}

func (server *Server) navigateBrowserSession(ctx *gin.Context) {
	var request dto.NavigateBrowserRequest
	if !server.bindJSON(ctx, &request) {
		return
	}
	if strings.TrimSpace(request.URL) == "" {
		invalidRequest(ctx, "url is required")
		return
	}
	client, ok := server.driving(ctx)
	if !ok {
		return
	}
	_, err := client.Navigate(ctx.Request.Context(), &browserservicepb.NavigateRequest{
		SessionId: ctx.Param("session_id"),
		Url:       request.URL,
		Referer:   request.Referer,
	})
	writeCommandResult(ctx, err)
}

func (server *Server) waitForBrowserNavigation(ctx *gin.Context) {
	var request dto.WaitForBrowserNavigationRequest
	if !server.bindJSON(ctx, &request) {
		return
	}
	waitUntil, err := parseWaitUntil(request.WaitUntil)
	if err != nil {
		invalidRequest(ctx, err.Error())
		return
	}
	client, ok := server.driving(ctx)
	if !ok {
		return
	}
	_, err = client.WaitForNavigation(
		ctx.Request.Context(), &browserservicepb.WaitForNavigationRequest{
			SessionId: ctx.Param("session_id"),
			WaitUntil: waitUntil,
			TimeoutMs: request.TimeoutMs,
		},
	)
	writeCommandResult(ctx, err)
}

// getBrowserNode reads an element. A GET because it is a read of the page, and
// the selector is short enough to live in the query string.
func (server *Server) getBrowserNode(ctx *gin.Context) {
	selector := strings.TrimSpace(ctx.Query("selector"))
	if selector == "" {
		invalidQuery(ctx, "selector is required")
		return
	}
	timeout, ok := queryMilliseconds(ctx, "timeout_ms")
	if !ok {
		return
	}
	client, valid := server.driving(ctx)
	if !valid {
		return
	}
	found, err := client.GetNode(ctx.Request.Context(), &browserservicepb.GetNodeRequest{
		SessionId: ctx.Param("session_id"),
		Selector:  selector,
		TimeoutMs: timeout,
	})
	if err != nil {
		writeCommandError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, nodeResponse(found.GetNode()))
}

func (server *Server) moveBrowserMouse(ctx *gin.Context) {
	var request dto.BrowserPointerRequest
	if !server.bindJSON(ctx, &request) {
		return
	}
	if !aimed(ctx, request) {
		return
	}
	client, ok := server.driving(ctx)
	if !ok {
		return
	}
	_, err := client.HumanMouseMove(
		ctx.Request.Context(), &browserservicepb.HumanMouseMoveRequest{
			SessionId: ctx.Param("session_id"),
			X:         request.X,
			Y:         request.Y,
			Selector:  request.Selector,
		},
	)
	writeCommandResult(ctx, err)
}

func (server *Server) clickBrowserMouse(ctx *gin.Context) {
	var request dto.ClickBrowserRequest
	if !server.bindJSON(ctx, &request) {
		return
	}
	button, err := parseMouseButton(request.Button)
	if err != nil {
		invalidRequest(ctx, err.Error())
		return
	}
	client, ok := server.driving(ctx)
	if !ok {
		return
	}
	_, err = client.HumanMouseClick(
		ctx.Request.Context(), &browserservicepb.HumanMouseClickRequest{
			SessionId: ctx.Param("session_id"),
			X:         request.X,
			Y:         request.Y,
			Selector:  request.Selector,
			Button:    button,
			Count:     request.Count,
			DelayMs:   request.DelayMs,
		},
	)
	writeCommandResult(ctx, err)
}

func (server *Server) typeIntoBrowser(ctx *gin.Context) {
	var request dto.TypeIntoBrowserRequest
	if !server.bindJSON(ctx, &request) {
		return
	}
	if request.Text == "" {
		invalidRequest(ctx, "text is required")
		return
	}
	client, ok := server.driving(ctx)
	if !ok {
		return
	}
	_, err := client.HumanType(ctx.Request.Context(), &browserservicepb.HumanTypeRequest{
		SessionId:  ctx.Param("session_id"),
		Text:       request.Text,
		Selector:   request.Selector,
		DelayMinMs: request.DelayMinMs,
		DelayMaxMs: request.DelayMaxMs,
	})
	writeCommandResult(ctx, err)
}

func (server *Server) scrollBrowser(ctx *gin.Context) {
	var request dto.ScrollBrowserRequest
	if !server.bindJSON(ctx, &request) {
		return
	}
	client, ok := server.driving(ctx)
	if !ok {
		return
	}
	_, err := client.HumanScrollY(ctx.Request.Context(), &browserservicepb.HumanScrollYRequest{
		SessionId: ctx.Param("session_id"),
		DeltaY:    request.DeltaY,
	})
	writeCommandResult(ctx, err)
}

func (server *Server) scrollBrowserTo(ctx *gin.Context) {
	var request dto.ScrollBrowserToRequest
	if !server.bindJSON(ctx, &request) {
		return
	}
	if strings.TrimSpace(request.Selector) == "" {
		invalidRequest(ctx, "selector is required")
		return
	}
	align, err := parseScrollAlign(request.Align)
	if err != nil {
		invalidRequest(ctx, err.Error())
		return
	}
	client, ok := server.driving(ctx)
	if !ok {
		return
	}
	_, err = client.HumanScrollYTo(
		ctx.Request.Context(), &browserservicepb.HumanScrollYToRequest{
			SessionId: ctx.Param("session_id"),
			Selector:  request.Selector,
			Align:     align,
		},
	)
	writeCommandResult(ctx, err)
}

func (server *Server) getBrowserCookies(ctx *gin.Context) {
	client, ok := server.driving(ctx)
	if !ok {
		return
	}
	captured, err := client.GetCookies(
		ctx.Request.Context(), &browserservicepb.GetCookiesRequest{
			SessionId: ctx.Param("session_id"),
		},
	)
	if err != nil {
		writeCommandError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.BrowserCookiesResponse{
		Cookies: service.DomainCookies(captured.GetCookies()),
	})
}

func (server *Server) setBrowserCookies(ctx *gin.Context) {
	var request dto.BrowserCookiesRequest
	if !server.bindJSON(ctx, &request) {
		return
	}
	client, ok := server.driving(ctx)
	if !ok {
		return
	}
	_, err := client.SetCookies(ctx.Request.Context(), &browserservicepb.SetCookiesRequest{
		SessionId: ctx.Param("session_id"),
		Cookies:   service.ServiceCookies(request.Cookies),
	})
	writeCommandResult(ctx, err)
}

// driving authorizes the command against the session in the path and returns
// the client to relay it with. It writes the failure itself, so a handler only
// has to stop.
func (server *Server) driving(
	ctx *gin.Context,
) (browserservicepb.BrowserServiceClient, bool) {
	if !server.browsersDriveable(ctx) {
		return nil, false
	}
	client, err := server.browserDriver.Driving(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		ctx.Param("session_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return nil, false
	}
	return client, true
}

func (server *Server) browsersDriveable(ctx *gin.Context) bool {
	if server.browserDriver != nil {
		return true
	}
	writeProblem(ctx, http.StatusServiceUnavailable, dto.Problem{
		Code:    "browser_sessions_unavailable",
		Message: "browsers are not configured on this server",
	})
	return false
}

// aimed refuses a pointer command that names nowhere to go. A click may leave
// the pointer where it is; a move to nowhere means nothing.
func aimed(ctx *gin.Context, request dto.BrowserPointerRequest) bool {
	if strings.TrimSpace(request.Selector) != "" {
		return true
	}
	if request.X != nil && request.Y != nil {
		return true
	}
	invalidRequest(ctx, "a pointer move needs a selector, or an x and a y")
	return false
}

// writeCommandResult answers a command that returns nothing.
func writeCommandResult(ctx *gin.Context, err error) {
	if err != nil {
		writeCommandError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

// writeCommandError gives the browser's own answer its HTTP shape.
//
// The browser is what refused, not this hop, so its code is what is translated:
// a selector that matched nothing is a 404 and not a 502, and a wait that ran
// out is a 504. Only a failure with no answer in it at all is this server
// failing to reach the browser.
func writeCommandError(ctx *gin.Context, err error) {
	answered, ok := status.FromError(err)
	if !ok {
		writeProblem(ctx, http.StatusBadGateway, dto.Problem{
			Code:    "browser_unreachable",
			Message: "the browser could not be reached",
		})
		return
	}
	switch answered.Code() {
	case codes.NotFound:
		writeProblem(ctx, http.StatusNotFound, dto.Problem{
			Code: "browser_element_not_found", Message: answered.Message(),
		})
	case codes.InvalidArgument:
		invalidRequest(ctx, answered.Message())
	case codes.DeadlineExceeded:
		writeProblem(ctx, http.StatusGatewayTimeout, dto.Problem{
			Code: "browser_timed_out", Message: answered.Message(),
		})
	case codes.Unimplemented:
		writeProblem(ctx, http.StatusNotImplemented, dto.Problem{
			Code: "browser_command_unsupported", Message: answered.Message(),
		})
	default:
		writeProblem(ctx, http.StatusBadGateway, dto.Problem{
			Code: "browser_failed", Message: answered.Message(),
		})
	}
}

func nodeResponse(found *browserservicepb.Node) dto.BrowserNodeResponse {
	attributes := make(map[string]string, len(found.GetAttributes()))
	for _, attribute := range found.GetAttributes() {
		attributes[attribute.GetName()] = attribute.GetValue()
	}
	return dto.BrowserNodeResponse{
		NodeID:     found.GetNodeId(),
		LocalName:  found.GetLocalName(),
		NodeType:   found.GetNodeType(),
		Attributes: attributes,
		Text:       found.GetText(),
		HTML:       found.GetHtml(),
		X:          found.GetX(),
		Y:          found.GetY(),
		Width:      found.GetWidth(),
		Height:     found.GetHeight(),
	}
}

// The three enums, spelled the way JSON spells things. Absent is the default in
// every case, which is the browser's own default and not one invented here.

func parseWaitUntil(value string) (browserservicepb.WaitUntil, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "load":
		return browserservicepb.WaitUntil_WAIT_UNTIL_LOAD, nil
	case "commit":
		return browserservicepb.WaitUntil_WAIT_UNTIL_COMMIT, nil
	case "dom_content_loaded":
		return browserservicepb.WaitUntil_WAIT_UNTIL_DOM_CONTENT_LOADED, nil
	case "network_almost_idle":
		return browserservicepb.WaitUntil_WAIT_UNTIL_NETWORK_ALMOST_IDLE, nil
	case "network_idle":
		return browserservicepb.WaitUntil_WAIT_UNTIL_NETWORK_IDLE, nil
	}
	return 0, errors.New(
		"wait_until must be commit, dom_content_loaded, load, " +
			"network_almost_idle or network_idle",
	)
}

func parseMouseButton(value string) (browserservicepb.MouseButton, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "left":
		return browserservicepb.MouseButton_MOUSE_BUTTON_LEFT, nil
	case "middle":
		return browserservicepb.MouseButton_MOUSE_BUTTON_MIDDLE, nil
	case "right":
		return browserservicepb.MouseButton_MOUSE_BUTTON_RIGHT, nil
	case "back":
		return browserservicepb.MouseButton_MOUSE_BUTTON_BACK, nil
	case "forward":
		return browserservicepb.MouseButton_MOUSE_BUTTON_FORWARD, nil
	}
	return 0, errors.New("button must be left, middle, right, back or forward")
}

func parseScrollAlign(value string) (browserservicepb.ScrollAlign, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "center":
		return browserservicepb.ScrollAlign_SCROLL_ALIGN_CENTER, nil
	case "top":
		return browserservicepb.ScrollAlign_SCROLL_ALIGN_TOP, nil
	case "bottom":
		return browserservicepb.ScrollAlign_SCROLL_ALIGN_BOTTOM, nil
	}
	return 0, errors.New("align must be top, center or bottom")
}

// queryMilliseconds reads an optional millisecond query parameter. Absent is
// zero, which every command reads as "the browser's own default".
func queryMilliseconds(ctx *gin.Context, name string) (uint32, bool) {
	raw := strings.TrimSpace(ctx.Query(name))
	if raw == "" {
		return 0, true
	}
	parsed, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		invalidQuery(ctx, name+" must be a whole number of milliseconds")
		return 0, false
	}
	return uint32(parsed), true
}

// writeBrowserDriverError maps a driver failure onto its HTTP shape.
func writeBrowserDriverError(ctx *gin.Context, err error) bool {
	switch {
	case errors.Is(err, browser.ErrUnavailable):
		writeProblem(ctx, http.StatusBadGateway, dto.Problem{
			Code:    "browser_unreachable",
			Message: "the browser could not be reached",
		})
	default:
		return false
	}
	return true
}
