//go:build integration

package store_test

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/argus-env/argus/internal/api"
	"github.com/argus-env/argus/internal/database"
	"github.com/argus-env/argus/internal/secrets"
	"github.com/argus-env/argus/internal/store"
)

func TestStoreLifecycle(t *testing.T) {
	ctx := t.Context()
	pool, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	migrations := make(chan error, 2)
	go func() { migrations <- database.Migrate(ctx, pool) }()
	go func() { migrations <- database.Migrate(ctx, pool) }()
	for range 2 {
		if err := <-migrations; err != nil {
			t.Fatal(err)
		}
	}
	cipher, err := secrets.New(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("i", 32))))
	if err != nil {
		t.Fatal(err)
	}
	data := store.New(pool, cipher)

	stamp := time.Now().UnixNano()
	token, err := data.CreateSession(ctx, stamp, "integration-user")
	if err != nil {
		t.Fatal(err)
	}
	userID, err := data.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	user, err := data.CurrentUser(ctx, userID)
	if err != nil || user.Username != "integration-user" {
		t.Fatalf("user=%#v err=%v", user, err)
	}
	project, environment, err := data.InitProject(ctx, userID, api.InitProjectRequest{Name: "integration-project", Environment: "dev", Variables: map[string]string{"FIRST": "one"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.DestroyProject(ctx, userID, project.ID) })
	if environment.Name != "dev" {
		t.Fatalf("unexpected environment: %#v", environment)
	}
	exists, err := data.EnvironmentExists(ctx, userID, project.ID, "dev")
	if err != nil || !exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
	if _, err := data.Push(ctx, userID, project.ID, "prod", map[string]string{"SECOND": "two"}); err != nil {
		t.Fatal(err)
	}
	if err := data.Set(ctx, userID, project.ID, "prod", "THIRD", "three"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.Push(ctx, userID, project.ID, "prod", map[string]string{"SECOND": "two"}); err != nil {
		t.Fatal(err)
	}
	values, err := data.Get(ctx, userID, project.ID, "prod", false)
	if err != nil || values["THIRD"] != "" || values["SECOND"] != "two" {
		t.Fatalf("values after exact push=%#v err=%v", values, err)
	}
	if err := data.Set(ctx, userID, project.ID, "prod", "THIRD", "three"); err != nil {
		t.Fatal(err)
	}
	values, err = data.Get(ctx, userID, project.ID, "prod", true)
	if err != nil {
		t.Fatal(err)
	}
	if values["SECOND"] != "two" || values["THIRD"] != "three" {
		t.Fatalf("unexpected values: %#v", values)
	}
	if err := data.DeleteVariable(ctx, userID, project.ID, "prod", "THIRD"); err != nil {
		t.Fatal(err)
	}
	values, err = data.Get(ctx, userID, project.ID, "prod", false)
	if err != nil || values["THIRD"] != "" || values["SECOND"] != "two" {
		t.Fatalf("values after delete=%#v err=%v", values, err)
	}
	projects, err := data.List(ctx, userID)
	if err != nil || len(projects) != 1 {
		t.Fatalf("projects=%#v err=%v", projects, err)
	}
	memberToken, err := data.CreateSession(ctx, stamp+1, "invited-user")
	if err != nil {
		t.Fatal(err)
	}
	memberID, err := data.Authenticate(ctx, memberToken)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := data.Invite(ctx, userID, project.ID, "invited-user", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	invitations, err := data.Invitations(ctx, memberID)
	if err != nil || len(invitations) != 1 || invitations[0].ID != invitation.ID {
		t.Fatalf("invitations=%#v err=%v", invitations, err)
	}
	if err := data.RespondInvitation(ctx, memberID, invitation.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := data.Push(ctx, memberID, project.ID, "prod", map[string]string{"SECOND": "blocked"}); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("viewer push error=%v", err)
	}
	if err := data.UpdateMemberRole(ctx, userID, project.ID, "invited-user", "member"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.Push(ctx, memberID, project.ID, "prod", map[string]string{"SECOND": "member-write"}); err != nil {
		t.Fatal(err)
	}
	members, err := data.Members(ctx, userID, project.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members=%#v err=%v", members, err)
	}
	if err := data.RemoveMember(ctx, userID, project.ID, "invited-user"); err != nil {
		t.Fatal(err)
	}
	events, err := data.History(ctx, userID, project.ID)
	if err != nil || len(events) < 6 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	if err := data.RemoveEnvironment(ctx, userID, project.ID, "dev"); err != nil {
		t.Fatal(err)
	}
	if err := data.DestroyProject(ctx, userID, project.ID); err != nil {
		t.Fatal(err)
	}
	if err := data.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
}
