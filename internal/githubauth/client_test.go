package githubauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeviceFlow(t *testing.T) {
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/login/device/code":
			_ = json.NewEncoder(writer).Encode(Device{DeviceCode: "device", UserCode: "ABCD-EFGH", VerificationURI: "https://github.test/device", Interval: 5})
		case "/login/oauth/access_token":
			polls++
			if polls == 1 {
				_ = json.NewEncoder(writer).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{"access_token": "github-token"})
		case "/user":
			if request.Header.Get("Authorization") != "Bearer github-token" {
				t.Errorf("unexpected authorization: %s", request.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(writer).Encode(User{ID: 42, Login: "octocat"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client, err := New("client-id")
	if err != nil {
		t.Fatal(err)
	}
	client.webURL, client.apiURL, client.http = server.URL, server.URL, server.Client()
	device, err := client.Start(t.Context())
	if err != nil || device.DeviceCode != "device" {
		t.Fatalf("start = %#v, %v", device, err)
	}
	if _, pending, err := client.Poll(t.Context(), device.DeviceCode); err != nil || !pending {
		t.Fatalf("first poll pending=%v err=%v", pending, err)
	}
	token, pending, err := client.Poll(t.Context(), device.DeviceCode)
	if err != nil || pending || token != "github-token" {
		t.Fatalf("second poll token=%q pending=%v err=%v", token, pending, err)
	}
	user, err := client.User(t.Context(), token)
	if err != nil || user.Login != "octocat" {
		t.Fatalf("user=%#v err=%v", user, err)
	}
}

func TestNewRequiresClientID(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected missing client id to fail")
	}
}
