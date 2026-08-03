package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const registryFile = "argus.toml"

type Project struct {
	Directory   string `toml:"directory"`
	ProjectID   string `toml:"project_id"`
	ProjectName string `toml:"project_name"`
	Environment string `toml:"environment,omitempty"`
}

type registry struct {
	Version  int       `toml:"version"`
	Projects []Project `toml:"projects"`
}

// Path returns the user-wide Argus project registry path for the current
// user. ARGUS_DATA_HOME is supported for tests and portable installations.
func Path() (string, error) {
	if override := os.Getenv("ARGUS_DATA_HOME"); override != "" {
		return filepath.Join(override, registryFile), nil
	}
	if runtime.GOOS == "windows" {
		root := os.Getenv("LOCALAPPDATA")
		if root == "" {
			var err error
			root, err = os.UserConfigDir()
			if err != nil {
				return "", fmt.Errorf("locate Windows application data: %w", err)
			}
		}
		return filepath.Join(root, "Argus", registryFile), nil
	}
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home directory: %w", err)
		}
		root = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(root, "argus", registryFile), nil
}

func Load(directory string) (Project, error) {
	canonical, err := canonicalDirectory(directory)
	if err != nil {
		return Project{}, err
	}
	contents, err := loadRegistry()
	if err != nil {
		return Project{}, err
	}
	for _, project := range contents.Projects {
		if samePath(project.Directory, canonical) {
			if project.ProjectID == "" || project.ProjectName == "" {
				return Project{}, errors.New("invalid Argus registry entry: project id and name are required")
			}
			return project, nil
		}
	}
	return Project{}, fmt.Errorf("directory %q is not registered: %w", canonical, os.ErrNotExist)
}

func Save(directory string, project Project) error {
	if project.ProjectID == "" || project.ProjectName == "" {
		return errors.New("project id and name are required")
	}
	canonical, err := canonicalDirectory(directory)
	if err != nil {
		return err
	}
	contents, err := loadRegistry()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if contents.Version == 0 {
		contents.Version = 1
	}
	project.Directory = canonical
	updated := false
	for index := range contents.Projects {
		if samePath(contents.Projects[index].Directory, canonical) {
			contents.Projects[index] = project
			updated = true
			break
		}
	}
	if !updated {
		contents.Projects = append(contents.Projects, project)
	}
	return writeRegistry(contents)
}

// Remove deletes only the entry for directory. The shared registry and all
// other project associations are preserved.
func Remove(directory string) error {
	canonical, err := canonicalDirectory(directory)
	if err != nil {
		return err
	}
	contents, err := loadRegistry()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	projects := contents.Projects[:0]
	for _, project := range contents.Projects {
		if !samePath(project.Directory, canonical) {
			projects = append(projects, project)
		}
	}
	if len(projects) == len(contents.Projects) {
		return nil
	}
	contents.Projects = projects
	return writeRegistry(contents)
}

// RemoveProject deletes every local directory association for a remote
// project. This is used after destroying a project by name from any directory.
func RemoveProject(projectID string) error {
	if projectID == "" {
		return errors.New("project id is required")
	}
	contents, err := loadRegistry()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	projects := contents.Projects[:0]
	for _, project := range contents.Projects {
		if project.ProjectID != projectID {
			projects = append(projects, project)
		}
	}
	if len(projects) == len(contents.Projects) {
		return nil
	}
	contents.Projects = projects
	return writeRegistry(contents)
}

func loadRegistry() (registry, error) {
	path, err := Path()
	if err != nil {
		return registry{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return registry{}, err
	}
	var contents registry
	if err := toml.Unmarshal(data, &contents); err != nil {
		return registry{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return contents, nil
}

func writeRegistry(contents registry) error {
	path, err := Path()
	if err != nil {
		return err
	}
	sort.Slice(contents.Projects, func(i, j int) bool { return contents.Projects[i].Directory < contents.Projects[j].Directory })
	data, err := toml.Marshal(contents)
	if err != nil {
		return fmt.Errorf("encode Argus registry: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Argus data directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".argus-*.toml")
	if err != nil {
		return fmt.Errorf("create temporary Argus registry: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("replace Argus registry: %w", err)
		}
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install Argus registry: %w", err)
	}
	return nil
}

func canonicalDirectory(directory string) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve project directory: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	}
	return filepath.Clean(absolute), nil
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
