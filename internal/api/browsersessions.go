package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/neurun-io/neurun/internal/browserservicepb"
	"github.com/neurun-io/neurun/internal/domain/browser"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/service"
)

// displayBufferBytes is a framebuffer update's working size. RFB sends large
// rectangles; a small buffer turns one update into many syscalls.
const displayBufferBytes = 32 << 10

func (server *Server) listBrowserSessions(ctx *gin.Context) {
	records, err := server.browserSessions.List(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"browser_sessions": dto.NewBrowserSessionResponses(records),
	})
}

func (server *Server) getBrowserSession(ctx *gin.Context) {
	record, err := server.browserSessions.Get(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		ctx.Param("session_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewBrowserSessionResponse(record))
}

func (server *Server) closeBrowserSession(ctx *gin.Context) {
	if err := server.browserSessions.Close(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		ctx.Param("session_id"),
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

// streamBrowserDisplay pipes the session's VNC server to the operator.
//
// The framebuffer is a logged-in browser rendered as pixels, so the credential
// is checked before the handshake completes rather than after: a refused viewer
// never sees a frame. The VNC port itself is on loopback and unauthenticated —
// this handler is the only thing that reaches it, and the only thing that asks
// who is watching.
func (server *Server) streamBrowserDisplay(ctx *gin.Context) {
	record, err := server.browserSessions.Get(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		ctx.Param("session_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	// Opened before the upgrade: once the connection is a WebSocket there is no
	// way left to answer with a problem document.
	stream, release, err := server.browserSessions.OpenDisplay(
		ctx.Request.Context(), record,
	)
	if err != nil {
		writeProblem(ctx, http.StatusBadGateway, dto.Problem{
			Code:    "display_unreachable",
			Message: "the session's display did not answer",
		})
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  displayBufferBytes,
		WriteBufferSize: displayBufferBytes,
		// noVNC asks for the binary RFB subprotocol by name.
		Subprotocols: []string{"binary"},
		CheckOrigin:  server.allowedOrigin,
	}
	socket, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		release()
		return
	}
	pipeDisplay(socket, stream, release, record.ID)
}

// allowedOrigin refuses a cross-site upgrade from anywhere but the dashboard.
// A WebSocket handshake is not covered by CORS, so without this any page could
// open one against a signed-in operator's cookie.
func (server *Server) allowedOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		// A non-browser client sends none. It still had to present a credential.
		return true
	}
	for _, allowed := range server.allowedOrigins {
		if allowed == origin {
			return true
		}
	}
	return false
}

// pipeDisplay copies RFB between the operator's socket and the browser
// service's stream. Neither end is told about the other: the viewer holds a
// session id, the service holds a framebuffer, and this is the only thing that
// has both.
func pipeDisplay(
	socket *websocket.Conn,
	stream service.DisplayStream,
	release func(),
	sessionID string,
) {
	defer socket.Close()
	defer release()

	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			chunk, err := stream.Recv()
			if err != nil {
				return
			}
			if len(chunk.GetData()) == 0 {
				continue
			}
			if err := socket.WriteMessage(
				websocket.BinaryMessage, chunk.GetData(),
			); err != nil {
				return
			}
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			kind, payload, err := socket.ReadMessage()
			if err != nil {
				return
			}
			if kind != websocket.BinaryMessage || len(payload) == 0 {
				continue
			}
			if err := stream.Send(&browserservicepb.DisplayChunk{Data: payload}); err != nil {
				return
			}
		}
	}()
	<-done
	slog.Debug("browser display stream ended", "session", sessionID)
}

// writeBrowserSessionError maps session failures onto their HTTP shape.
func writeBrowserSessionError(ctx *gin.Context, err error) bool {
	switch {
	case errors.Is(err, browser.ErrSessionNotFound):
		notFound(ctx, "browser session")
	default:
		return false
	}
	return true
}
