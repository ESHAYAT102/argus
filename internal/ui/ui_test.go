package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/argus-env/argus/internal/api"
)

func TestActivityLog(t *testing.T) {
	var output bytes.Buffer
	ActivityLog(&output, []api.Activity{
		{Action: "environment.pushed", Actor: "octocat", Environment: "prod", CreatedAt: time.Date(2026, time.August, 4, 12, 34, 0, 0, time.Local)},
		{Action: "variable.changed", Actor: "octocat", Environment: "prod", Variable: "DATABASE_URL", CreatedAt: time.Date(2026, time.August, 4, 12, 35, 0, 0, time.Local)},
	})

	for _, want := range []string{"Aug 04 12:34", "DONE", "Environment pushed", "actor=@octocat", "env=prod", "EDIT", "Variable changed", "variable=DATABASE_URL"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("ActivityLog() output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestActivityLogEmptyState(t *testing.T) {
	var output bytes.Buffer
	ActivityLog(&output, nil)
	if output.String() != "Nothing has happened here yet.\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestMembersTable(t *testing.T) {
	var output bytes.Buffer
	MembersTable(&output, []api.Member{{Username: "octocat", Role: "member"}})
	for _, want := range []string{"Member", "Role", "@octocat", "member"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output does not contain %q: %s", want, output.String())
		}
	}
}

func TestInvitationsEmptyState(t *testing.T) {
	var output bytes.Buffer
	InvitationsTable(&output, nil)
	if output.String() != "You don't have any pending invitations.\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestInvitationsTable(t *testing.T) {
	var output bytes.Buffer
	InvitationsTable(&output, []api.Invitation{{ID: "invite-id", Project: "portfolio", Inviter: "owner", Role: "viewer", ExpiresAt: time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local)}})
	for _, want := range []string{"invite-id", "portfolio", "@owner", "viewer", "Aug 11"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output does not contain %q: %s", want, output.String())
		}
	}
}

func TestProjectsTable(t *testing.T) {
	var output bytes.Buffer
	ProjectsTable(&output, []struct{ Name, Environments string }{
		{Name: "SASS", Environments: "dev, prod"},
		{Name: "portfolio", Environments: "prod"},
	})

	for _, want := range []string{"Projects", "Environments", "SASS", "dev, prod", "portfolio", "prod", "╭", "╯"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("ProjectsTable() output does not contain %q:\n%s", want, output.String())
		}
	}

	lines := strings.Split(output.String(), "\n")
	if !strings.HasPrefix(lines[1], "│ Projects") {
		t.Errorf("header has unexpected left padding: %q", lines[1])
	}
	if !strings.HasPrefix(lines[3], "│ SASS") {
		t.Errorf("first row has unexpected left padding: %q", lines[3])
	}
}

func TestProjectsTableWithNoProjects(t *testing.T) {
	var output bytes.Buffer
	ProjectsTable(&output, nil)

	for _, want := range []string{"Projects", "Environments"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("ProjectsTable() output does not contain %q:\n%s", want, output.String())
		}
	}
}
