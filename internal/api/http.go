package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"github.com/dagflows/neurun-io/internal/ids"
)

type Problem struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type ErrorEnvelope struct {
	Error     Problem `json:"error"`
	RequestID string  `json:"request_id,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("encode HTTP response", "error", err)
	}
}

func WriteProblem(w http.ResponseWriter, request *http.Request, status int, problem Problem) {
	WriteJSON(w, status, ErrorEnvelope{
		Error:     problem,
		RequestID: RequestID(request.Context()),
	})
}

func DecodeJSON(w http.ResponseWriter, request *http.Request, destination any, maximumBytes int64) bool {
	if maximumBytes <= 0 {
		maximumBytes = 1 << 20
	}
	contentType := request.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			WriteProblem(w, request, http.StatusUnsupportedMediaType, Problem{
				Code:    "unsupported_media_type",
				Message: "Content-Type must be application/json",
			})
			return false
		}
	}

	request.Body = http.MaxBytesReader(w, request.Body, maximumBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		status := http.StatusBadRequest
		code := "invalid_json"
		message := "request body is not valid JSON"
		var maximum *http.MaxBytesError
		if errors.As(err, &maximum) {
			status = http.StatusRequestEntityTooLarge
			code = "request_too_large"
			message = "request body exceeds the configured limit"
		}
		WriteProblem(w, request, status, Problem{
			Code:    code,
			Message: message,
			Details: map[string]any{"cause": err.Error()},
		})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		WriteProblem(w, request, http.StatusBadRequest, Problem{
			Code:    "invalid_json",
			Message: "request body must contain exactly one JSON value",
		})
		return false
	}
	return true
}

type requestIDKey struct{}

func RequestID(ctx interface{ Value(any) any }) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestID := strings.TrimSpace(request.Header.Get("Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			generated, err := ids.New("req")
			if err != nil {
				WriteJSON(w, http.StatusInternalServerError, ErrorEnvelope{
					Error: Problem{Code: "internal_error", Message: "could not allocate request ID"},
				})
				return
			}
			requestID = generated
		}
		w.Header().Set("Request-ID", requestID)
		next.ServeHTTP(w, request.WithContext(withRequestID(request.Context(), requestID)))
	})
}

func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("panic in HTTP handler",
					"request_id", RequestID(request.Context()),
					"method", request.Method,
					"path", request.URL.Path,
					"panic", recovered,
				)
				WriteProblem(w, request, http.StatusInternalServerError, Problem{
					Code:    "internal_error",
					Message: "the server could not complete the request",
				})
			}
		}()
		next.ServeHTTP(w, request)
	})
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, request)
	})
}
