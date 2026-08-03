package ui

import (
	"bytes"
	"strings"
	"testing"
)

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
