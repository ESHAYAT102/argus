package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type SessionStore struct{}

func (SessionStore) Load() (string, error) {
	data, err := os.ReadFile(sessionPath())
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("empty session")
	}
	return token, nil
}

func (SessionStore) Save(token string) error {
	directory := filepath.Dir(sessionPath())
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	return os.WriteFile(sessionPath(), []byte(token+"\n"), 0o600)
}

func (SessionStore) Clear() error {
	err := os.Remove(sessionPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func sessionPath() string {
	directory, err := os.UserConfigDir()
	if err != nil {
		directory = os.TempDir()
	}
	return filepath.Join(directory, "argus", "session")
}
