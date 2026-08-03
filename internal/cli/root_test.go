package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/argus-env/argus/internal/api"
)

type authenticationClient struct {
	api.Client
	user              api.User
	whoamiErr         error
	authenticateCalls int
}

func (client *authenticationClient) WhoAmI(context.Context) (api.User, error) {
	return client.user, client.whoamiErr
}

func (client *authenticationClient) Authenticate(context.Context) error {
	client.authenticateCalls++
	return nil
}

func TestGetMissingEnvironmentError(t *testing.T) {
	app := &application{}
	root := app.rootCommand()
	command, _, err := root.Find([]string{"get"})
	if err != nil {
		t.Fatal(err)
	}
	err = command.Args(command, nil)
	if err == nil {
		t.Fatal("expected missing environment to fail")
	}
	want := "missing environment name\n\nUsage:\n  argus get <environment>\n\nExamples:\n  argus get dev\n  argus get prod"
	if err.Error() != want {
		t.Fatalf("error:\n%s\n\nwant:\n%s", err, want)
	}
}

func TestSetMissingVariableError(t *testing.T) {
	app := &application{}
	root := app.rootCommand()
	command, _, err := root.Find([]string{"set"})
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Args(command, nil); err == nil || !strings.Contains(err.Error(), "missing variable name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintErrorUsesReadablePrefix(t *testing.T) {
	var output bytes.Buffer
	app := &application{errOut: &output}
	app.printError(errors.New("something went wrong"))
	if got := output.String(); !strings.Contains(got, "Error: something went wrong") {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestPushCommandReplacesSync(t *testing.T) {
	root := (&application{}).rootCommand()
	foundPush := false
	for _, command := range root.Commands() {
		switch command.Name() {
		case "push":
			foundPush = true
		case "sync":
			t.Fatal("sync command should no longer be registered")
		}
	}
	if !foundPush {
		t.Fatal("push command is not registered")
	}
}

func TestAuthDoesNotAuthenticateTwice(t *testing.T) {
	var output bytes.Buffer
	client := &authenticationClient{user: api.User{Username: "octocat"}}
	app := &application{client: client, out: &output}
	command := app.authCommand()
	if err := command.RunE(command, nil); err != nil {
		t.Fatal(err)
	}
	if client.authenticateCalls != 0 {
		t.Fatalf("Authenticate called %d times", client.authenticateCalls)
	}
	if got := output.String(); got != "Already authenticated as octocat.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestAuthStartsWhenSessionIsMissing(t *testing.T) {
	client := &authenticationClient{whoamiErr: api.ErrUnauthenticated}
	app := &application{client: client, out: &bytes.Buffer{}}
	command := app.authCommand()
	if err := command.RunE(command, nil); err != nil {
		t.Fatal(err)
	}
	if client.authenticateCalls != 1 {
		t.Fatalf("Authenticate called %d times", client.authenticateCalls)
	}
}

func TestWhoAmIPrintsUsername(t *testing.T) {
	var output bytes.Buffer
	client := &authenticationClient{user: api.User{Username: "octocat"}}
	app := &application{client: client, out: &output}
	command := app.whoamiCommand()
	if err := command.RunE(command, nil); err != nil {
		t.Fatal(err)
	}
	if output.String() != "octocat\n" {
		t.Fatalf("output = %q", output.String())
	}
}
