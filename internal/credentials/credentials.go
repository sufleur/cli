// Package credentials manages the on-disk store for user API keys obtained via
// `sufleur login`. The file lives at $XDG_CONFIG_HOME/sufleur/credentials.yaml
// (default $HOME/.config/sufleur/credentials.yaml), with 0600 permissions.
package credentials

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	fileMode = 0o600
	dirMode  = 0o700
	fileName = "credentials.yaml"
	subdir   = "sufleur"
)

// Credentials is the persisted user-authentication state.
type Credentials struct {
	APIBase string `yaml:"api_base"`
	APIKey  string `yaml:"api_key"`
	UserID  string `yaml:"user_id"`
	KeyID   string `yaml:"key_id"`
}

// Path returns the absolute path to the credentials file, honouring
// XDG_CONFIG_HOME. It does not check whether the file exists.
func Path() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, subdir, fileName), nil
}

// Load reads and parses the credentials file. Returns os.ErrNotExist (wrapped)
// if the file is absent — callers should check with errors.Is.
func Load() (*Credentials, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading credentials: %w", err)
	}
	var c Credentials
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}
	return &c, nil
}

// Exists reports whether the credentials file is present on disk.
func Exists() (bool, error) {
	path, err := Path()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// Save writes c to the credentials file, creating the parent directory if
// needed. The file is written with 0600 permissions.
func Save(c Credentials) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}
	if err := os.WriteFile(path, data, fileMode); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}
	return nil
}

// Delete removes the credentials file. Returns nil if the file is already gone.
func Delete() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing credentials: %w", err)
	}
	return nil
}
