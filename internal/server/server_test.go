package server

import "testing"

func TestValidGitHubUsername(t *testing.T) {
	for _, username := range []string{"octocat", "ESHAYAT102", "user-name"} {
		if !validGitHubUsername(username) {
			t.Errorf("%q should be valid", username)
		}
	}
	for _, username := range []string{"", "-user", "user-", "user--name", "user_name", "user name"} {
		if validGitHubUsername(username) {
			t.Errorf("%q should be invalid", username)
		}
	}
}

func TestValidateResourceName(t *testing.T) {
	if err := validateResourceName("project", "valid"); err != nil {
		t.Fatal(err)
	}
	if err := validateResourceName("project", ""); err == nil {
		t.Fatal("expected empty name to fail")
	}
	if err := validateResourceName("environment", string(make([]byte, 101))); err == nil {
		t.Fatal("expected long name to fail")
	}
}
