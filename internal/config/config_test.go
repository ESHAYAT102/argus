package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultUnixRegistryPath(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Unix path assertion")
	}
	home := t.TempDir()
	t.Setenv("ARGUS_DATA_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", home)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "share", "argus", "argus.toml")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestRegistryStoresMultipleProjectsByDirectory(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("ARGUS_DATA_HOME", dataHome)
	firstDirectory := t.TempDir()
	secondDirectory := t.TempDir()
	first := Project{ProjectID: "prj_123", ProjectName: "portfolio", Environment: "prod"}
	second := Project{ProjectID: "prj_456", ProjectName: "saas", Environment: "dev"}
	if err := Save(firstDirectory, first); err != nil {
		t.Fatal(err)
	}
	if err := Save(secondDirectory, second); err != nil {
		t.Fatal(err)
	}

	gotFirst, err := Load(firstDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if gotFirst.ProjectID != first.ProjectID || gotFirst.ProjectName != first.ProjectName || gotFirst.Environment != first.Environment {
		t.Fatalf("got %#v, want %#v", gotFirst, first)
	}
	gotSecond, err := Load(secondDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if gotSecond.ProjectID != second.ProjectID {
		t.Fatalf("got %#v, want %#v", gotSecond, second)
	}

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dataHome, "argus.toml") {
		t.Fatalf("path = %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestRemovePreservesOtherProjects(t *testing.T) {
	t.Setenv("ARGUS_DATA_HOME", t.TempDir())
	firstDirectory, secondDirectory := t.TempDir(), t.TempDir()
	if err := Save(firstDirectory, Project{ProjectID: "one", ProjectName: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(secondDirectory, Project{ProjectID: "two", ProjectName: "two"}); err != nil {
		t.Fatal(err)
	}
	if err := Remove(firstDirectory); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(firstDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed project error = %v", err)
	}
	if _, err := Load(secondDirectory); err != nil {
		t.Fatalf("other project was removed: %v", err)
	}
}

func TestSaveUpdatesExistingDirectory(t *testing.T) {
	t.Setenv("ARGUS_DATA_HOME", t.TempDir())
	directory := t.TempDir()
	if err := Save(directory, Project{ProjectID: "one", ProjectName: "project", Environment: "dev"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(directory, Project{ProjectID: "one", ProjectName: "project", Environment: "prod"}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got.Environment != "prod" {
		t.Fatalf("environment = %q", got.Environment)
	}
}

func TestRemoveProjectRemovesEveryMatchingDirectory(t *testing.T) {
	t.Setenv("ARGUS_DATA_HOME", t.TempDir())
	firstDirectory, secondDirectory, remainingDirectory := t.TempDir(), t.TempDir(), t.TempDir()
	if err := Save(firstDirectory, Project{ProjectID: "shared", ProjectName: "shared"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(secondDirectory, Project{ProjectID: "shared", ProjectName: "shared"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(remainingDirectory, Project{ProjectID: "remaining", ProjectName: "remaining"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveProject("shared"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(firstDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first mapping remains: %v", err)
	}
	if _, err := Load(secondDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second mapping remains: %v", err)
	}
	if _, err := Load(remainingDirectory); err != nil {
		t.Fatalf("unrelated mapping was removed: %v", err)
	}
}
