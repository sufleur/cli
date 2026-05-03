package lockfile

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Lockfile represents sufleur-lock.yaml.
type Lockfile struct {
	Resolved map[string]ResolvedPrompt `yaml:"resolved"`
}

// ResolvedPrompt captures a pinned prompt version.
//
// Package is the underlying registry reference when this entry is an alias
// for another prompt; for non-aliased entries it is left empty so the
// on-disk YAML stays as compact as it has historically been.
type ResolvedPrompt struct {
	Package      string    `yaml:"package,omitempty"`
	Version      string    `yaml:"version"`
	IntegritySHA string    `yaml:"integrity_sha"`
	Constraint   string    `yaml:"constraint"`
	Status       string    `yaml:"status"`
	ResolvedAt   time.Time `yaml:"resolved_at"`
}

// NewLockfile creates an empty lockfile.
func NewLockfile() *Lockfile {
	return &Lockfile{
		Resolved: make(map[string]ResolvedPrompt),
	}
}

// Load reads a lockfile from disk.
func Load(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading lockfile: %w", err)
	}

	var lf Lockfile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parsing lockfile: %w", err)
	}

	if lf.Resolved == nil {
		lf.Resolved = make(map[string]ResolvedPrompt)
	}

	return &lf, nil
}

// Save writes the lockfile to disk.
func Save(path string, lf *Lockfile) error {
	data, err := yaml.Marshal(lf)
	if err != nil {
		return fmt.Errorf("marshaling lockfile: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
