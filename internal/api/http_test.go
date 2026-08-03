package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testTokenStore struct{}

func (testTokenStore) Load() (string, error) { return "token", nil }
func (testTokenStore) Save(string) error     { return nil }
func (testTokenStore) Clear() error          { return nil }

func TestPresentDeviceAuthorizationCopiesThenOpens(t *testing.T) {
	var output bytes.Buffer
	copied, opened := "", ""
	client := &HTTPClient{
		input: strings.NewReader("\n"), output: &output,
		copy: func(value string) error { copied = value; return nil },
		open: func(value string) error { opened = value; return nil },
	}
	device := deviceAuth{UserCode: "EACA-64BA", VerificationURI: "https://github.com/login/device"}
	if err := client.presentDeviceAuthorization(device); err != nil {
		t.Fatal(err)
	}
	if copied != device.UserCode {
		t.Fatalf("copied %q", copied)
	}
	if opened != device.VerificationURI {
		t.Fatalf("opened %q", opened)
	}
	want := "Copied to clipboard: EACA-64BA\nPress Enter to open https://github.com/login/device in your browser...\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestPresentDeviceAuthorizationFallsBackGracefully(t *testing.T) {
	var output bytes.Buffer
	client := &HTTPClient{
		input: strings.NewReader("\n"), output: &output,
		copy: func(string) error { return errors.New("unavailable") },
		open: func(string) error { return errors.New("unavailable") },
	}
	device := deviceAuth{UserCode: "EACA-64BA", VerificationURI: "https://github.com/login/device"}
	if err := client.presentDeviceAuthorization(device); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "copy it from below") || !strings.Contains(output.String(), "Open https://github.com/login/device manually") {
		t.Fatalf("missing fallback guidance: %q", output.String())
	}
}

func TestPresentDeviceAuthorizationRequiresEnter(t *testing.T) {
	client := &HTTPClient{input: strings.NewReader(""), output: &bytes.Buffer{}, copy: func(string) error { return nil }, open: func(string) error { t.Fatal("browser should not open"); return nil }}
	err := client.presentDeviceAuthorization(deviceAuth{UserCode: "CODE", VerificationURI: "https://github.com/login/device"})
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInspectDisablesActivityRecording(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/projects/project/environments/prod/variables" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.URL.Query().Get("record_activity") != "false" {
			t.Fatalf("query = %q", request.URL.RawQuery)
		}
		_, _ = io.WriteString(writer, `{"variables":{"PORT":"3000"}}`)
	}))
	defer server.Close()
	client := NewHTTPClient(server.URL, testTokenStore{})
	values, err := client.Inspect(context.Background(), "project", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if values["PORT"] != "3000" {
		t.Fatalf("values = %#v", values)
	}
}

func TestDeleteVariableRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/v1/projects/project/environments/prod/variables/OLD_KEY" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := NewHTTPClient(server.URL, testTokenStore{})
	if err := client.DeleteVariable(context.Background(), "project", "prod", "OLD_KEY"); err != nil {
		t.Fatal(err)
	}
}

func TestProjectAccessRequests(t *testing.T) {
	tests := []struct {
		name, method, path string
		call               func(*HTTPClient) error
	}{
		{name: "share", method: http.MethodPost, path: "/v1/projects/project/invitations", call: func(client *HTTPClient) error {
			_, err := client.ShareProject(context.Background(), "project", "octocat", "member")
			return err
		}},
		{name: "members", method: http.MethodGet, path: "/v1/projects/project/members", call: func(client *HTTPClient) error { _, err := client.Members(context.Background(), "project"); return err }},
		{name: "role", method: http.MethodPatch, path: "/v1/projects/project/members/octocat", call: func(client *HTTPClient) error {
			return client.UpdateMemberRole(context.Background(), "project", "octocat", "viewer")
		}},
		{name: "unshare", method: http.MethodDelete, path: "/v1/projects/project/members/octocat", call: func(client *HTTPClient) error { return client.RemoveMember(context.Background(), "project", "octocat") }},
		{name: "invites", method: http.MethodGet, path: "/v1/invitations", call: func(client *HTTPClient) error { _, err := client.Invitations(context.Background()); return err }},
		{name: "accept", method: http.MethodPost, path: "/v1/invitations/invite/accept", call: func(client *HTTPClient) error { return client.AcceptInvitation(context.Background(), "invite") }},
		{name: "decline", method: http.MethodPost, path: "/v1/invitations/invite/decline", call: func(client *HTTPClient) error { return client.DeclineInvitation(context.Background(), "invite") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != test.method || request.URL.Path != test.path {
					t.Fatalf("request = %s %s", request.Method, request.URL.Path)
				}
				if test.name == "share" {
					_, _ = io.WriteString(writer, `{"id":"invite","role":"member","expires_at":"2026-08-11T00:00:00Z"}`)
					return
				}
				if test.name == "members" {
					_, _ = io.WriteString(writer, `{"members":[]}`)
					return
				}
				if test.name == "invites" {
					_, _ = io.WriteString(writer, `{"invitations":[]}`)
					return
				}
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			if err := test.call(NewHTTPClient(server.URL, testTokenStore{})); err != nil {
				t.Fatal(err)
			}
		})
	}
}
