package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/domain/auth"
	"github.com/neurun-io/neurun/internal/domain/operator"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/service"
)

// SessionCookieName carries the operator session token.
//
// The token is a bearer secret, so the cookie is HttpOnly (script cannot read
// it), SameSite=Strict (it does not ride cross-site requests), and Secure
// wherever the deployment is not plain-HTTP localhost.
const SessionCookieName = "neurun_operator_session"

// operatorPrincipal maps a session onto the same Principal the API-key path
// produces, so scope enforcement has exactly one implementation.
func operatorPrincipal(session operator.Session) auth.Principal {
	return auth.Principal{
		Kind:           auth.KindOperator,
		OperatorID:     session.AccountID,
		Email:          session.Email,
		OrganizationID: session.OrganizationID,
		Scopes:         session.Role.Scopes(),
	}
}

func (server *Server) sessionCookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   server.operatorCookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
}

func (server *Server) clearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   server.operatorCookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
}

func sessionToken(ctx *gin.Context) string {
	cookie, err := ctx.Request.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func (server *Server) operatorLogin(ctx *gin.Context) {
	if server.operators == nil {
		writeProblem(ctx, http.StatusServiceUnavailable, dto.Problem{
			Code:    "operator_signin_unavailable",
			Message: "operator sign-in is not configured on this server",
		})
		return
	}
	var body dto.LoginRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	email := strings.TrimSpace(body.Email)
	if email == "" || body.Password == "" {
		writeProblem(ctx, http.StatusBadRequest, dto.Problem{
			Code: "invalid_request", Message: "email and password are required",
		})
		return
	}
	session, token, err := server.operators.Login(
		ctx.Request.Context(), email, body.Password,
	)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			// One message for unknown user, wrong password and disabled account
			// alike: the response must not be usable to enumerate accounts.
			writeProblem(ctx, http.StatusUnauthorized, dto.Problem{
				Code: "invalid_credentials", Message: "invalid email or password",
			})
			return
		}
		writeProblem(ctx, http.StatusInternalServerError, dto.Problem{
			Code: "internal_error", Message: "could not complete sign-in",
		})
		return
	}
	http.SetCookie(ctx.Writer, server.sessionCookie(token, session.ExpiresAt))
	ctx.JSON(http.StatusOK, gin.H{"operator": dto.NewOperatorResponse(session)})
}

func (server *Server) operatorLogout(ctx *gin.Context) {
	// Always clear the cookie and always answer 204, whether or not the token
	// was live. Signing out must not report whether a session existed.
	if server.operators != nil {
		if token := sessionToken(ctx); token != "" {
			_ = server.operators.Logout(ctx.Request.Context(), token)
		}
	}
	http.SetCookie(ctx.Writer, server.clearSessionCookie())
	ctx.Status(http.StatusNoContent)
}

func (server *Server) operatorSession(ctx *gin.Context) {
	if server.operators == nil {
		writeProblem(ctx, http.StatusServiceUnavailable, dto.Problem{
			Code:    "operator_signin_unavailable",
			Message: "no operator accounts are configured",
		})
		return
	}
	token := sessionToken(ctx)
	session, err := server.operators.Session(ctx.Request.Context(), token)
	if err != nil {
		if token != "" {
			http.SetCookie(ctx.Writer, server.clearSessionCookie())
		}
		unauthenticated(ctx, "not signed in")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"operator": dto.NewOperatorResponse(session)})
}
