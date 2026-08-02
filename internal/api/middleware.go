package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/domain/auth"
	"github.com/neurun-io/neurun/internal/domain/operator"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/ids"
)

const requestIDKey = "neurun_request_id"

func requestIDOf(ctx *gin.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

// requestID echoes a caller-supplied Request-ID or mints one, so a client and
// the server logs can name the same request.
func requestID() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		value := strings.TrimSpace(ctx.GetHeader("Request-ID"))
		if value == "" || len(value) > 128 {
			generated, err := ids.New("req")
			if err != nil {
				ctx.AbortWithStatusJSON(http.StatusInternalServerError, dto.ErrorResponse{
					Error: dto.Problem{
						Code: "internal_error", Message: "could not allocate request ID",
					},
				})
				return
			}
			value = generated
		}
		ctx.Set(requestIDKey, value)
		ctx.Header("Request-ID", value)
		ctx.Next()
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Header("X-Content-Type-Options", "nosniff")
		ctx.Header("X-Frame-Options", "DENY")
		ctx.Header("Referrer-Policy", "no-referrer")
		ctx.Header("Cache-Control", "no-store")
		ctx.Next()
	}
}

// recovery turns a panicking handler into a 500 rather than a dropped
// connection, and records enough to find it afterwards.
func recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(ctx *gin.Context, recovered any) {
		slog.Error("panic in HTTP handler",
			"request_id", requestIDOf(ctx),
			"method", ctx.Request.Method,
			"path", ctx.Request.URL.Path,
			"panic", recovered,
		)
		writeProblem(ctx, http.StatusInternalServerError, dto.Problem{
			Code:    "internal_error",
			Message: "the server could not complete the request",
		})
	})
}

// authenticate accepts either a bearer API key or an operator session cookie.
//
// The bearer header wins when both are present: an Authorization header is a
// deliberate act by a caller, whereas a cookie rides along automatically.
func (server *Server) authenticate() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if raw, ok := auth.BearerToken(ctx.GetHeader("Authorization")); ok {
			principal, valid := server.accounts.AuthenticateContext(ctx.Request.Context(), raw)
			if !valid {
				unauthenticated(ctx, "the supplied API key was rejected")
				return
			}
			server.withPrincipal(ctx, principal)
			return
		}

		token := sessionToken(ctx)
		if token == "" {
			unauthenticated(ctx, "sign in, or supply a bearer API key")
			return
		}
		if server.operators == nil {
			unauthenticated(ctx, "operator sign-in is not configured on this server")
			return
		}
		session, err := server.operators.Session(ctx.Request.Context(), token)
		if err != nil {
			// Clear an expired or unknown session so the browser stops resending
			// a token that will never work again.
			http.SetCookie(ctx.Writer, server.clearSessionCookie())
			message := "your session is no longer valid; sign in again"
			if errors.Is(err, operator.ErrSessionExpired) {
				message = "your session expired; sign in again"
			}
			unauthenticated(ctx, message)
			return
		}
		server.withPrincipal(ctx, operatorPrincipal(session))
	}
}

func (server *Server) withPrincipal(ctx *gin.Context, principal auth.Principal) {
	ctx.Request = ctx.Request.WithContext(
		auth.WithPrincipal(ctx.Request.Context(), principal),
	)
	ctx.Next()
}

// scoped rejects a principal that does not carry the required scope.
func (server *Server) scoped(scope string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		principal, ok := auth.FromContext(ctx.Request.Context())
		if !ok {
			unauthenticated(ctx, "a valid bearer API key is required")
			return
		}
		if !principal.HasScope(scope) {
			writeProblem(ctx, http.StatusForbidden, dto.Problem{
				Code:    "permission_denied",
				Message: "the API key does not grant the required scope",
				Details: map[string]any{"required_scope": scope},
			})
			return
		}
		ctx.Next()
	}
}

func unauthenticated(ctx *gin.Context, message string) {
	ctx.Header("WWW-Authenticate", `Bearer realm="neurun"`)
	writeProblem(ctx, http.StatusUnauthorized, dto.Problem{
		Code: "unauthorized", Message: message,
	})
}

// bindJSON decodes exactly one JSON value into destination, rejecting unknown
// fields and anything past the configured size.
func (server *Server) bindJSON(ctx *gin.Context, destination any) bool {
	if contentType := ctx.GetHeader("Content-Type"); contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			writeProblem(ctx, http.StatusUnsupportedMediaType, dto.Problem{
				Code:    "unsupported_media_type",
				Message: "Content-Type must be application/json",
			})
			return false
		}
	}
	ctx.Request.Body = http.MaxBytesReader(
		ctx.Writer, ctx.Request.Body, server.maximumBodyBytes,
	)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		status, code := http.StatusBadRequest, "invalid_json"
		message := "request body is not valid JSON"
		var maximum *http.MaxBytesError
		if errors.As(err, &maximum) {
			status, code = http.StatusRequestEntityTooLarge, "request_too_large"
			message = "request body exceeds the configured limit"
		}
		writeProblem(ctx, status, dto.Problem{
			Code: code, Message: message,
			Details: map[string]any{"cause": err.Error()},
		})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(ctx, http.StatusBadRequest, dto.Problem{
			Code:    "invalid_json",
			Message: "request body must contain exactly one JSON value",
		})
		return false
	}
	return true
}

func requireEmptyBody(ctx *gin.Context) bool {
	if ctx.Request.Body == nil || ctx.Request.ContentLength == 0 {
		return true
	}
	invalidRequest(ctx, "request body must be empty")
	return false
}
