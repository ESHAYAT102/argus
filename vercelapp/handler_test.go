package vercelapp

import (
	"net/http/httptest"
	"testing"
)

func TestRoutedPath(t *testing.T) {
	for input, expected := range map[string]string{
		"health":          "/health",
		"v1/projects":     "/v1/projects",
		"/v1/auth/logout": "/v1/auth/logout",
	} {
		request := httptest.NewRequest("GET", "/api?__argus_path="+input, nil)
		actual, err := routedPath(request)
		if err != nil {
			t.Fatalf("route %q: %v", input, err)
		}
		if actual != expected {
			t.Fatalf("route %q = %q, want %q", input, actual, expected)
		}
	}
}

func TestRoutedPathRejectsUnknownRoute(t *testing.T) {
	request := httptest.NewRequest("GET", "/api?__argus_path=admin", nil)
	if _, err := routedPath(request); err == nil {
		t.Fatal("expected unknown route to fail")
	}
}
