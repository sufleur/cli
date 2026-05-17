package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPath_XDGOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(dir, "sufleur", "credentials.yaml")
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestPath_DefaultHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", dir)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(dir, ".config", "sufleur", "credentials.yaml")
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	original := Credentials{
		APIBase: "https://api.sufleur.com",
		APIKey:  "u_abcd1234567890abcdef",
		UserID:  "00000000-0000-0000-0000-000000000001",
		KeyID:   "11111111-1111-1111-1111-111111111111",
	}
	if err := Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *loaded != original {
		t.Errorf("loaded = %+v, want %+v", *loaded, original)
	}
}

func TestSave_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file permissions not enforced on Windows")
	}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := Save(Credentials{APIBase: "x", APIKey: "y", UserID: "z", KeyID: "k"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := fi.Mode().Perm(); mode != 0o600 {
		t.Errorf("file mode = %o, want 0600", mode)
	}

	parent := filepath.Dir(path)
	pfi, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("Stat parent: %v", err)
	}
	if mode := pfi.Mode().Perm(); mode != 0o700 {
		t.Errorf("dir mode = %o, want 0700", mode)
	}
}

func TestSave_CreatesMissingDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "deep", "nested"))

	if err := Save(Credentials{APIBase: "x", APIKey: "y", UserID: "z", KeyID: "k"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := Load(); err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
}

func TestLoad_Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want wrapped os.ErrNotExist", err)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	got, err := Exists()
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if got {
		t.Error("Exists = true on empty dir, want false")
	}

	if err := Save(Credentials{APIBase: "x", APIKey: "y", UserID: "z", KeyID: "k"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err = Exists()
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !got {
		t.Error("Exists = false after Save, want true")
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Deleting when missing should not error.
	if err := Delete(); err != nil {
		t.Fatalf("Delete on missing: %v", err)
	}

	if err := Save(Credentials{APIBase: "x", APIKey: "y", UserID: "z", KeyID: "k"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	exists, err := Exists()
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("file still exists after Delete")
	}
}
