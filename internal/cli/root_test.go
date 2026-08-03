package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/argus-env/argus/internal/api"
	"github.com/argus-env/argus/internal/config"
	"github.com/argus-env/argus/internal/dotenv"
	"github.com/argus-env/argus/internal/project"
)

type authenticationClient struct {
	api.Client
	user              api.User
	whoamiErr         error
	authenticateCalls int
	logoutCalls       int
}

type projectListClient struct {
	api.Client
	projects []api.Project
}

type comparisonClient struct {
	api.Client
	remote map[string]string
}

type deletionClient struct {
	api.Client
	err    error
	called bool
}

type pullClient struct {
	api.Client
	environments map[string]map[string]string
}

func (client *pullClient) Get(_ context.Context, _, environment string) (map[string]string, error) {
	return client.environments[environment], nil
}

func (client *deletionClient) DeleteVariable(context.Context, string, string, string) error {
	client.called = true
	return client.err
}

func (client *comparisonClient) Inspect(context.Context, string, string) (map[string]string, error) {
	return client.remote, nil
}

func (client *projectListClient) List(context.Context) ([]api.Project, error) {
	return client.projects, nil
}

func (client *authenticationClient) WhoAmI(context.Context) (api.User, error) {
	return client.user, client.whoamiErr
}

func (client *authenticationClient) Authenticate(context.Context) error {
	client.authenticateCalls++
	return nil
}

func (client *authenticationClient) Logout(context.Context) error {
	client.logoutCalls++
	return nil
}

func TestPullMissingEnvironmentError(t *testing.T) {
	app := &application{}
	root := app.rootCommand()
	command, _, err := root.Find([]string{"pull"})
	if err != nil {
		t.Fatal(err)
	}
	err = command.Args(command, nil)
	if err == nil {
		t.Fatal("expected missing environment to fail")
	}
	want := "missing environment name\n\nUsage:\n  argus pull <environment>\n\nExamples:\n  argus pull dev\n  argus pull prod"
	if err.Error() != want {
		t.Fatalf("error:\n%s\n\nwant:\n%s", err, want)
	}
}

func TestCompareVariablesDoesNotExposeValues(t *testing.T) {
	difference := compareVariables(
		map[string]string{"LOCAL": "local-secret", "CHANGED": "old-secret", "SAME": "same"},
		map[string]string{"REMOTE": "remote-secret", "CHANGED": "new-secret", "SAME": "same"},
	)
	if strings.Join(difference.LocalOnly, ",") != "LOCAL" || strings.Join(difference.Changed, ",") != "CHANGED" || strings.Join(difference.RemoteOnly, ",") != "REMOTE" {
		t.Fatalf("difference = %#v", difference)
	}
}

func TestDiffPrintsNamesWithoutValues(t *testing.T) {
	t.Setenv("ARGUS_DATA_HOME", t.TempDir())
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte("CHANGED=local-secret\nLOCAL=only-here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(directory, config.Project{ProjectID: "project-id", ProjectName: "demo", Environment: "dev"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	app := &application{client: &comparisonClient{remote: map[string]string{"CHANGED": "remote-secret", "REMOTE": "only-there"}}, out: &output, cwd: func() (string, error) { return directory, nil }}
	command := app.diffCommand()
	if err := command.RunE(command, []string{"dev"}); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, name := range []string{"CHANGED", "LOCAL", "REMOTE"} {
		if !strings.Contains(got, name) {
			t.Fatalf("output does not contain %q: %s", name, got)
		}
	}
	for _, secret := range []string{"local-secret", "remote-secret", "only-here", "only-there"} {
		if strings.Contains(got, secret) {
			t.Fatalf("output leaked %q: %s", secret, got)
		}
	}
}

func TestProjectLinkSavesGlobalAssociation(t *testing.T) {
	t.Setenv("ARGUS_DATA_HOME", t.TempDir())
	directory := t.TempDir()
	client := &projectListClient{projects: []api.Project{{ID: "project-id", Name: "portfolio", Environments: []api.Environment{{Name: "prod"}}}}}
	app := &application{client: client, out: &bytes.Buffer{}, cwd: func() (string, error) { return directory, nil }, discoverProject: func(string) (project.Discovery, error) { return project.Discovery{}, errors.New("not a repository") }}
	command := app.projectLinkCommand()
	if err := command.RunE(command, []string{"Portfolio"}); err != nil {
		t.Fatal(err)
	}
	linked, err := config.Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	if linked.ProjectID != "project-id" || linked.Environment != "prod" {
		t.Fatalf("linked = %#v", linked)
	}
	if _, err := os.Stat(filepath.Join(directory, ".argus.toml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected project-local config: %v", err)
	}
}

func TestDeleteUpdatesRemoteBeforeLocalFile(t *testing.T) {
	for _, test := range []struct {
		name       string
		remoteErr  error
		shouldKeep bool
	}{
		{name: "success", shouldKeep: false},
		{name: "remote failure", remoteErr: errors.New("offline"), shouldKeep: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ARGUS_DATA_HOME", t.TempDir())
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, ".env"), []byte("DELETE_ME=secret\nKEEP=yes\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := config.Save(directory, config.Project{ProjectID: "project-id", ProjectName: "demo", Environment: "prod"}); err != nil {
				t.Fatal(err)
			}
			client := &deletionClient{err: test.remoteErr}
			app := &application{
				client: client,
				out:    &bytes.Buffer{},
				cwd:    func() (string, error) { return directory, nil },
				confirmPrompt: func(string) (bool, error) {
					return true, nil
				},
			}
			command := app.deleteCommand()
			err := command.RunE(command, []string{"DELETE_ME"})
			if !client.called {
				t.Fatal("remote delete was not called")
			}
			if test.remoteErr != nil && !errors.Is(err, test.remoteErr) {
				t.Fatalf("error = %v", err)
			}
			values, readErr := dotenv.Read(directory)
			if readErr != nil {
				t.Fatal(readErr)
			}
			_, kept := values["DELETE_ME"]
			if kept != test.shouldKeep || values["KEEP"] != "yes" {
				t.Fatalf("values = %#v", values)
			}
		})
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

func TestDestroyRequiresProjectName(t *testing.T) {
	app := &application{}
	command := app.destroyCommand()
	if err := command.Args(command, nil); err == nil || !strings.Contains(err.Error(), "missing project name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDestroyTargetDoesNotUseCurrentDirectory(t *testing.T) {
	client := &projectListClient{projects: []api.Project{{ID: "project-id", Name: "portfolio"}}}
	app := &application{
		client: client,
		cwd: func() (string, error) {
			t.Fatal("destroy should not inspect the current directory")
			return "", nil
		},
	}

	metadata, err := app.destroyTarget(context.Background(), "Portfolio")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ProjectID != "project-id" || metadata.ProjectName != "portfolio" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestListShowsEmptyState(t *testing.T) {
	var output bytes.Buffer
	app := &application{
		client: &projectListClient{},
		out:    &output,
		cwd: func() (string, error) {
			t.Fatal("list should not inspect the current directory")
			return "", nil
		},
	}
	command := app.listCommand()

	if err := command.RunE(command, nil); err != nil {
		t.Fatal(err)
	}

	want := "You don't have any projects yet.\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
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

func TestPullCommandReplacesGet(t *testing.T) {
	root := (&application{}).rootCommand()
	foundPull := false
	for _, command := range root.Commands() {
		switch command.Name() {
		case "pull":
			foundPull = true
		case "get":
			t.Fatal("get command should no longer be registered")
		}
	}
	if !foundPull {
		t.Fatal("pull command is not registered")
	}
}

func TestPullBackupBehavior(t *testing.T) {
	for _, test := range []struct {
		name          string
		modifyBetween bool
		wantBackups   int
	}{
		{name: "untouched pulled file", wantBackups: 0},
		{name: "locally modified file", modifyBetween: true, wantBackups: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ARGUS_DATA_HOME", t.TempDir())
			directory := t.TempDir()
			if err := config.Save(directory, config.Project{ProjectID: "project-id", ProjectName: "demo"}); err != nil {
				t.Fatal(err)
			}
			app := &application{
				client: &pullClient{environments: map[string]map[string]string{
					"dev":  {"MODE": "development"},
					"prod": {"MODE": "production"},
				}},
				out: &bytes.Buffer{},
				cwd: func() (string, error) { return directory, nil },
				now: func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) },
			}
			if err := app.pullCommand().RunE(app.pullCommand(), []string{"dev"}); err != nil {
				t.Fatal(err)
			}
			if test.modifyBetween {
				if err := os.WriteFile(filepath.Join(directory, ".env"), []byte("MODE=custom-local-value\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := app.pullCommand().RunE(app.pullCommand(), []string{"prod"}); err != nil {
				t.Fatal(err)
			}
			backups, err := filepath.Glob(filepath.Join(directory, ".env.backup.*"))
			if err != nil {
				t.Fatal(err)
			}
			if len(backups) != test.wantBackups {
				t.Fatalf("backups = %v", backups)
			}
			values, err := dotenv.Read(directory)
			if err != nil || values["MODE"] != "production" {
				t.Fatalf("values=%#v err=%v", values, err)
			}
		})
	}
}

func TestWorkflowCommandsAreRegistered(t *testing.T) {
	root := (&application{}).rootCommand()
	for _, path := range [][]string{{"status"}, {"diff"}, {"delete"}, {"project", "link"}} {
		command, _, err := root.Find(path)
		if err != nil || command.Name() != path[len(path)-1] {
			t.Fatalf("command %v was not registered: command=%v err=%v", path, command, err)
		}
	}
}

func TestStatusReportsMatchingEnvironment(t *testing.T) {
	t.Setenv("ARGUS_DATA_HOME", t.TempDir())
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte("PORT=3000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(directory, config.Project{ProjectID: "project-id", ProjectName: "demo", Environment: "dev"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	app := &application{client: &comparisonClient{remote: map[string]string{"PORT": "3000"}}, out: &output, cwd: func() (string, error) { return directory, nil }}
	command := app.statusCommand()
	if err := command.RunE(command, nil); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "Project: demo") || !strings.Contains(got, "Environment: dev") || !strings.Contains(got, "in sync") {
		t.Fatalf("output = %q", got)
	}
}

func TestRootDoesNotIncludeCompletionCommand(t *testing.T) {
	root := (&application{}).rootCommand()
	for _, command := range root.Commands() {
		if command.Name() == "completion" {
			t.Fatal("completion command should not be registered")
		}
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

func TestAuthDoesNotInspectCurrentDirectory(t *testing.T) {
	client := &authenticationClient{user: api.User{Username: "octocat"}}
	app := &application{
		client: client,
		out:    &bytes.Buffer{},
		cwd: func() (string, error) {
			t.Fatal("auth should not inspect the current directory")
			return "", nil
		},
	}
	command := app.authCommand()
	if err := command.RunE(command, nil); err != nil {
		t.Fatal(err)
	}
}

func TestLogoutDoesNotInspectCurrentDirectory(t *testing.T) {
	client := &authenticationClient{}
	app := &application{
		client: client,
		out:    &bytes.Buffer{},
		cwd: func() (string, error) {
			t.Fatal("logout should not inspect the current directory")
			return "", nil
		},
	}
	command := app.logoutCommand()
	if err := command.RunE(command, nil); err != nil {
		t.Fatal(err)
	}
	if client.logoutCalls != 1 {
		t.Fatalf("Logout called %d times", client.logoutCalls)
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

func TestProjectContextMatchesGitHubRemoteAutomatically(t *testing.T) {
	t.Setenv("ARGUS_DATA_HOME", t.TempDir())
	repositoryRoot := t.TempDir()
	workingDirectory := repositoryRoot
	client := &projectListClient{projects: []api.Project{{ID: "project-id", Name: "nooreyah", Repository: "ESHAYAT102/nooreyah"}}}
	app := &application{
		client: client,
		cwd:    func() (string, error) { return workingDirectory, nil },
		discoverProject: func(string) (project.Discovery, error) {
			return project.Discovery{Root: repositoryRoot, Repository: "eshayat102/nooreyah", Name: "nooreyah"}, nil
		},
	}
	directory, metadata, err := app.projectContext(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if directory != repositoryRoot || metadata.ProjectID != "project-id" {
		t.Fatalf("directory=%q metadata=%#v", directory, metadata)
	}
	saved, err := config.Load(repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ProjectID != "project-id" {
		t.Fatalf("saved metadata=%#v", saved)
	}
}

func TestProjectContextExplainsUninitializedRepository(t *testing.T) {
	t.Setenv("ARGUS_DATA_HOME", t.TempDir())
	repositoryRoot := t.TempDir()
	app := &application{
		client: &projectListClient{},
		cwd:    func() (string, error) { return repositoryRoot, nil },
		discoverProject: func(string) (project.Discovery, error) {
			return project.Discovery{Root: repositoryRoot, Repository: "owner/repository", Name: "repository"}, nil
		},
	}
	_, _, err := app.projectContext(context.Background(), true)
	if err == nil || !strings.Contains(err.Error(), "GitHub repository \"owner/repository\" is not initialized") {
		t.Fatalf("unexpected error: %v", err)
	}
}
