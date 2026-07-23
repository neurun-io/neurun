package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name string `json:"name"`
	}
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var decoded payload
		if !DecodeJSON(w, request, &decoded, 128) {
			return
		}
		WriteJSON(w, http.StatusOK, decoded)
	}))

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"neurun"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	if response.Header().Get("Request-ID") == "" {
		t.Fatal("missing Request-ID")
	}
}

func TestDecodeJSONRejectsUnknownAndTrailingValues(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"unknown":  `{"extra":true}`,
		"trailing": `{} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				var decoded struct{}
				DecodeJSON(w, request, &decoded, 128)
			}))
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body)
			}
			var envelope ErrorEnvelope
			if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.RequestID == "" {
				t.Fatal("missing request ID in error")
			}
		})
	}
}

func TestDecodeJSONBoundsBody(t *testing.T) {
	t.Parallel()

	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var decoded map[string]any
		DecodeJSON(w, request, &decoded, 16)
	}))
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"message":"this is too large"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
}
