package project

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

type Discovery struct {
	Root       string
	Repository string
	Name       string
}

func Discover(directory string) (Discovery, error) {
	root, err := git(directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return Discovery{}, errors.New("current directory is not a Git repository")
	}
	remote, err := git(root, "remote", "get-url", "origin")
	if err != nil {
		return Discovery{}, errors.New("Git repository has no origin remote")
	}
	repository, err := normalizeGitHubRemote(remote)
	if err != nil {
		return Discovery{}, err
	}
	_, name := filepath.Split(repository)
	return Discovery{Root: root, Repository: repository, Name: name}, nil
}

func git(directory string, arguments ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func normalizeGitHubRemote(remote string) (string, error) {
	remote = strings.TrimSpace(strings.TrimSuffix(remote, ".git"))
	if strings.HasPrefix(remote, "git@github.com:") {
		path := strings.TrimPrefix(remote, "git@github.com:")
		if validRepositoryPath(path) {
			return path, nil
		}
	}
	parsed, err := url.Parse(remote)
	if err == nil && strings.EqualFold(parsed.Hostname(), "github.com") {
		path := strings.Trim(parsed.Path, "/")
		if validRepositoryPath(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("origin is not a valid GitHub repository: %s", remote)
}

func validRepositoryPath(path string) bool {
	parts := strings.Split(path, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}
