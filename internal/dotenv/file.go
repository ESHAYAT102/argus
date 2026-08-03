package dotenv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const Filename = ".env"

func Read(directory string) (map[string]string, error) {
	path := filepath.Join(directory, Filename)
	values, err := godotenv.Read(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s does not exist", path)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return values, nil
}

// WriteSafely writes values to .env. A non-empty existing file is moved to a
// timestamped backup before the replacement is installed.
func WriteSafely(directory string, values map[string]string, now time.Time) (backupPath string, err error) {
	return write(directory, values, now, true)
}

// Write replaces .env atomically without backing up the existing file.
func Write(directory string, values map[string]string) error {
	_, err := write(directory, values, time.Time{}, false)
	return err
}

func write(directory string, values map[string]string, now time.Time, backupExisting bool) (backupPath string, err error) {
	path := filepath.Join(directory, Filename)
	encoded, err := marshal(values)
	if err != nil {
		return "", err
	}

	temporary, err := os.CreateTemp(directory, ".argus-env-*")
	if err != nil {
		return "", fmt.Errorf("create temporary env file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("secure temporary env file: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write temporary env file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("sync temporary env file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close temporary env file: %w", err)
	}

	info, statErr := os.Stat(path)
	if backupExisting && statErr == nil && info.Size() > 0 {
		backupPath = uniqueBackupPath(directory, now)
		if err := os.Rename(path, backupPath); err != nil {
			return "", fmt.Errorf("back up existing .env: %w", err)
		}
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", fmt.Errorf("inspect existing .env: %w", statErr)
	}

	if err := os.Rename(temporaryPath, path); err != nil {
		if backupPath != "" {
			_ = os.Rename(backupPath, path)
		}
		return "", fmt.Errorf("install new .env: %w", err)
	}
	return backupPath, nil
}

// FileSignature identifies the current .env file using non-secret filesystem
// metadata. It lets the CLI recognize an untouched file from the last pull.
func FileSignature(directory string) (string, error) {
	info, err := os.Stat(filepath.Join(directory, Filename))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano()), nil
}

func Set(directory, name, value string) error {
	values := map[string]string{}
	path := filepath.Join(directory, Filename)
	if _, err := os.Stat(path); err == nil {
		loaded, err := godotenv.Read(path)
		if err != nil {
			return err
		}
		values = loaded
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	values[name] = value
	encoded, err := marshal(values)
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o600)
}

func Delete(directory, name string) (bool, error) {
	path := filepath.Join(directory, Filename)
	values, err := godotenv.Read(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if _, exists := values[name]; !exists {
		return false, nil
	}
	delete(values, name)
	encoded, err := marshal(values)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

func marshal(values map[string]string) ([]byte, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	ordered := make(map[string]string, len(values))
	for _, key := range keys {
		ordered[key] = values[key]
	}
	content, err := godotenv.Marshal(ordered)
	if err != nil {
		return nil, fmt.Errorf("encode environment variables: %w", err)
	}
	return []byte(content + "\n"), nil
}

func uniqueBackupPath(directory string, now time.Time) string {
	stem := filepath.Join(directory, Filename+".backup."+now.Format("20060102-150405"))
	path := stem
	for suffix := 2; ; suffix++ {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return path
		}
		path = fmt.Sprintf("%s-%d", stem, suffix)
	}
}

func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("variable name cannot be empty")
	}
	for index, character := range name {
		if !(character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || index > 0 && character >= '0' && character <= '9') {
			return fmt.Errorf("invalid variable name %q", name)
		}
	}
	return nil
}
