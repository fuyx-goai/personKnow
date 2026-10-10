package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"knowledge-base/internal/platform/httpx"
)

func TestV1StatusReportsMiniProgramCapabilitiesWithoutAuthentication(t *testing.T) {
	engine := New(nil, V1Handlers{Verifier: rejectingVerifier{}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Version      string   `json:"version"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal("decode status response:", err)
	}
	if response.Version != "v1" || len(response.Capabilities) == 0 {
		t.Fatalf("unexpected status response: %+v", response)
	}
}

func TestV1StatusReportsStandaloneModeWhenFullStackIsDisabled(t *testing.T) {
	engine := New(nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "miniprogram") || !strings.Contains(recorder.Body.String(), "standalone") {
		t.Fatalf("unexpected standalone status: %s", recorder.Body.String())
	}
}

func TestV1WechatLoginRouteIsRegistered(t *testing.T) {
	engine := New(nil, V1Handlers{Verifier: rejectingVerifier{}})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/wechat/login", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("login route is not registered: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

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
