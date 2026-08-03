package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const projectFile = ".argus.toml"

type Project struct {
	ProjectID   string
	ProjectName string
	Environment string
}

func Path(directory string) string {
	return filepath.Join(directory, projectFile)
}

func Load(directory string) (Project, error) {
	data, err := os.ReadFile(Path(directory))
	if err != nil {
		return Project{}, err
	}

	var project Project
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch strings.TrimSpace(key) {
		case "project_id":
			project.ProjectID = value
		case "project_name":
			project.ProjectName = value
		case "environment":
			project.Environment = value
		}
	}
	if project.ProjectID == "" {
		return Project{}, errors.New("invalid .argus.toml: project_id is missing")
	}
	return project, nil
}

func Save(directory string, project Project) error {
	if project.ProjectID == "" || project.ProjectName == "" {
		return errors.New("project id and name are required")
	}
	content := fmt.Sprintf("project_id = %q\nproject_name = %q\nenvironment = %q\n", project.ProjectID, project.ProjectName, project.Environment)
	return os.WriteFile(Path(directory), []byte(content), 0o600)
}
