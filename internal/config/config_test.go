package config

import "testing"

func TestProjectRoundTrip(t *testing.T) {
	directory := t.TempDir()
	want := Project{ProjectID: "prj_123", ProjectName: "portfolio", Environment: "prod"}
	if err := Save(directory, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
