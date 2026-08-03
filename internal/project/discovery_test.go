package project

import "testing"

func TestNormalizeGitHubRemote(t *testing.T) {
	tests := map[string]string{
		"git@github.com:openai/codex.git":   "openai/codex",
		"https://github.com/openai/codex":   "openai/codex",
		"ssh://git@github.com/openai/codex": "openai/codex",
	}
	for input, expected := range tests {
		actual, err := normalizeGitHubRemote(input)
		if err != nil {
			t.Errorf("normalize %q: %v", input, err)
			continue
		}
		if actual != expected {
			t.Errorf("normalize %q = %q, want %q", input, actual, expected)
		}
	}
}

func TestNormalizeGitHubRemoteRejectsOtherHosts(t *testing.T) {
	if _, err := normalizeGitHubRemote("https://example.com/owner/repo.git"); err == nil {
		t.Fatal("expected non-GitHub remote to be rejected")
	}
}
