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
