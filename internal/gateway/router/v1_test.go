package router

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"knowledge-base/internal/platform/httpx"
)

type rejectingVerifier struct{}

func (rejectingVerifier) VerifyAccess(string) (httpx.Principal, error) {
	return httpx.Principal{}, errors.New("invalid")
}

func TestV1ProtectedRouteUsesAuthAndRequestID(t *testing.T) {
	engine := New(nil, V1Handlers{Verifier: rejectingVerifier{}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get(httpx.RequestIDHeader) == "" {
		t.Fatal("request id header is missing")
	}
}

func TestV1CORSPreflight(t *testing.T) {
	engine := New(nil, V1Handlers{Verifier: rejectingVerifier{}})
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/libraries", nil)
	request.Header.Set("Origin", "https://example.com")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Fatalf("unexpected CORS origin: %s", recorder.Header().Get("Access-Control-Allow-Origin"))
	}
}
