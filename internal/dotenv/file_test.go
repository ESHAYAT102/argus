package dotenv

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteSafelyBacksUpExistingFile(t *testing.T) {
	directory := t.TempDir()
	original := []byte("OLD=value\n")
	if err := os.WriteFile(filepath.Join(directory, Filename), original, 0o600); err != nil {
		t.Fatal(err)
	}

	backup, err := WriteSafely(directory, map[string]string{"NEW": "secret"}, time.Date(2026, 8, 4, 14, 30, 52, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(backup) != ".env.backup.20260804-143052" {
		t.Fatalf("unexpected backup name: %s", backup)
	}
	backedUp, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(backedUp) != string(original) {
		t.Fatalf("backup changed: %q", backedUp)
	}
	values, err := Read(directory)
	if err != nil {
		t.Fatal(err)
	}
	if values["NEW"] != "secret" || len(values) != 1 {
		t.Fatalf("unexpected new values: %#v", values)
	}
}

func TestWriteSafelyDoesNotBackUpEmptyFile(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, Filename), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	backup, err := WriteSafely(directory, map[string]string{"A": "B"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" {
		t.Fatalf("unexpected backup: %s", backup)
	}
}

func TestValidateName(t *testing.T) {
	for _, valid := range []string{"A", "DATABASE_URL", "value2"} {
		if err := ValidateName(valid); err != nil {
			t.Errorf("%q should be valid: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "2FAST", "HAS-DASH", "WITH SPACE"} {
		if err := ValidateName(invalid); err == nil {
			t.Errorf("%q should be invalid", invalid)
		}
	}
}

func TestDeleteRemovesOnlyRequestedVariable(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, Filename), []byte("KEEP=yes\nREMOVE=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := Delete(directory, "REMOVE")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected variable to be removed")
	}
	values, err := Read(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values["KEEP"] != "yes" {
		t.Fatalf("values = %#v", values)
	}
}

func TestDeleteMissingVariableIsNoOp(t *testing.T) {
	directory := t.TempDir()
	removed, err := Delete(directory, "MISSING")
	if err != nil || removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
}
