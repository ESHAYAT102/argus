package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

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
