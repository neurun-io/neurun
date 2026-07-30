package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dagflows/neurun-io/internal/auth"
	"github.com/dagflows/neurun-io/internal/operator"
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
		Kind:       auth.KindOperator,
		OperatorID: session.AccountID,
		Username:   session.Username,
		ProjectID:  session.ProjectID,
		Scopes:     session.Role.Scopes(),
	}
}

func (s *Server) sessionCookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.operatorCookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
}

func (s *Server) clearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.operatorCookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
}

func sessionToken(request *http.Request) string {
	cookie, err := request.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

/* -------------------------------------------------------------------------- */
/* Authentication middleware                                                   */
/* -------------------------------------------------------------------------- */

// authenticate accepts either a bearer API key or an operator session cookie.
//
// The bearer header wins when both are present: an explicit Authorization header
// is a deliberate act by a caller, whereas a cookie rides along automatically.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if raw, ok := auth.BearerToken(request.Header.Get("Authorization")); ok {
			principal, valid := s.apiKeys.Authenticate(raw)
			if !valid {
				s.writeUnauthenticated(w, request, "the supplied API key was rejected")
				return
			}
			next.ServeHTTP(w, request.WithContext(
				auth.WithPrincipal(request.Context(), principal)))
			return
		}

		token := sessionToken(request)
		if token == "" {
			s.writeUnauthenticated(w, request,
				"sign in, or supply a bearer API key")
			return
		}
		if s.operators == nil {
			s.writeUnauthenticated(w, request, "operator sign-in is not configured on this server")
			return
		}

		session, err := s.operators.Session(request.Context(), token)
		if err != nil {
			// An expired or unknown session is cleared so the browser stops
			// resending a token that will never work again.
			http.SetCookie(w, s.clearSessionCookie())
			message := "your session is no longer valid; sign in again"
			if errors.Is(err, operator.ErrSessionExpired) {
				message = "your session expired; sign in again"
			}
			s.writeUnauthenticated(w, request, message)
			return
		}

		next.ServeHTTP(w, request.WithContext(
			auth.WithPrincipal(request.Context(), operatorPrincipal(session))))
	})
}

func (s *Server) writeUnauthenticated(w http.ResponseWriter, request *http.Request, message string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="neurun"`)
	WriteProblem(w, request, http.StatusUnauthorized, Problem{
		Code:    "unauthorized",
		Message: message,
	})
}

/* -------------------------------------------------------------------------- */
/* Handlers                                                                    */
/* -------------------------------------------------------------------------- */

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// operatorView is the safe projection of a session. It deliberately carries no
// token, no password material, and no API key.
type operatorView struct {
	OperatorID string    `json:"operator_id"`
	Username   string    `json:"username"`
	Role       string    `json:"role"`
	ProjectID  string    `json:"project_id"`
	Scopes     []string  `json:"scopes"`
	SessionID  string    `json:"session_id"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func viewOf(session operator.Session) operatorView {
	return operatorView{
		OperatorID: session.AccountID,
		Username:   session.Username,
		Role:       string(session.Role),
		ProjectID:  session.ProjectID,
		Scopes:     session.Role.Scopes(),
		SessionID:  session.ID,
		ExpiresAt:  session.ExpiresAt.UTC(),
	}
}

func (s *Server) operatorLogin(w http.ResponseWriter, request *http.Request) {
	if s.operators == nil {
		WriteProblem(w, request, http.StatusServiceUnavailable, Problem{
			Code: "operator_signin_unavailable",
			Message: "no operator accounts are configured; " +
				"set NEURUN_OPERATOR_ACCOUNTS to enable username and password sign-in",
		})
		return
	}

	var body loginRequest
	if !DecodeJSON(w, request, &body, s.maximumBodyBytes) {
		return
	}
	username := strings.TrimSpace(body.Username)
	if username == "" || body.Password == "" {
		WriteProblem(w, request, http.StatusBadRequest, Problem{
			Code:    "invalid_request",
			Message: "username and password are required",
		})
		return
	}

	session, token, err := s.operators.Login(request.Context(), username, body.Password)
	if err != nil {
		var lockedOut *operator.LockedOutError
		switch {
		case errors.As(err, &lockedOut):
			retryAfter := int(lockedOut.RetryAfter.Round(time.Second).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			WriteProblem(w, request, http.StatusTooManyRequests, Problem{
				Code:    "too_many_attempts",
				Message: lockedOut.Error(),
			})
		case errors.Is(err, operator.ErrInvalidCredentials):
			// One message for unknown user, wrong password and disabled account
			// alike: the response must not be usable to enumerate accounts.
			WriteProblem(w, request, http.StatusUnauthorized, Problem{
				Code:    "invalid_credentials",
				Message: "invalid username or password",
			})
		default:
			WriteProblem(w, request, http.StatusInternalServerError, Problem{
				Code:    "internal_error",
				Message: "could not complete sign-in",
			})
		}
		return
	}

	http.SetCookie(w, s.sessionCookie(token, session.ExpiresAt))
	WriteJSON(w, http.StatusOK, map[string]any{
		"operator":   viewOf(session),
		"request_id": RequestID(request.Context()),
	})
}

func (s *Server) operatorLogout(w http.ResponseWriter, request *http.Request) {
	// Always clear the cookie and always answer 204, whether or not the token
	// was live. Signing out must not report whether a session existed.
	if s.operators != nil {
		if token := sessionToken(request); token != "" {
			_ = s.operators.Logout(request.Context(), token)
		}
	}
	http.SetCookie(w, s.clearSessionCookie())
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) operatorSession(w http.ResponseWriter, request *http.Request) {
	if s.operators == nil {
		WriteProblem(w, request, http.StatusServiceUnavailable, Problem{
			Code:    "operator_signin_unavailable",
			Message: "no operator accounts are configured",
		})
		return
	}

	token := sessionToken(request)
	session, err := s.operators.Session(request.Context(), token)
	if err != nil {
		if token != "" {
			http.SetCookie(w, s.clearSessionCookie())
		}
		s.writeUnauthenticated(w, request, "not signed in")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"operator": viewOf(session)})
}
