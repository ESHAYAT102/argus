//go:build integration

package store_test

import (
	"encoding/base64"
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
	values, err := data.Get(ctx, userID, project.ID, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if values["SECOND"] != "two" || values["THIRD"] != "three" {
		t.Fatalf("unexpected values: %#v", values)
	}
	projects, err := data.List(ctx, userID)
	if err != nil || len(projects) != 1 {
		t.Fatalf("projects=%#v err=%v", projects, err)
	}
	events, err := data.History(ctx, userID, project.ID)
	if err != nil || len(events) < 5 {
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
