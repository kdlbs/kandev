package gocache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/system/storage"
)

func TestExecutionEnvironmentRejectsUnsafeSymlinks(t *testing.T) {
	for _, test := range []struct {
		name string
		path func(t *testing.T, home, target string) string
	}{
		{
			name: "adopted root",
			path: func(t *testing.T, home, target string) string {
				link := filepath.Join(home, "adopted-cache")
				if err := os.Symlink(target, link); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				return link
			},
		},
		{
			name: "managed ancestor",
			path: func(t *testing.T, home, target string) string {
				if err := os.Symlink(target, filepath.Join(home, "cache")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				return filepath.Join(home, "cache", "go-build")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			target := filepath.Join(t.TempDir(), "go-build")
			if err := os.MkdirAll(target, 0o700); err != nil {
				t.Fatal(err)
			}
			artifact := filepath.Join(target, "sentinel")
			if err := os.WriteFile(artifact, []byte("retain target"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := test.path(t, home, target)
			settings := storage.DefaultSettings()
			settings.GoCache.Enabled = true
			if test.name == "adopted root" {
				settings.GoCache.AdoptedPath = path
			}
			provider := New(Config{
				HomeDir: home, TrashDir: filepath.Join(home, "trash"),
				Settings: staticSettings{settings: settings},
			})

			if _, err := provider.ExecutionEnvironment(context.Background()); err == nil {
				t.Fatal("ExecutionEnvironment succeeded through an unsafe symlink")
			}
			if data, err := os.ReadFile(artifact); err != nil || string(data) != "retain target" {
				t.Fatalf("target data changed: data=%q err=%v", data, err)
			}
		})
	}
}

func TestCleanupRejectsAdoptedRootSymlink(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "scheduled", true: "explicit"}[explicit], func(t *testing.T) {
			home := t.TempDir()
			target := filepath.Join(t.TempDir(), "external-cache")
			if err := os.MkdirAll(target, 0o700); err != nil {
				t.Fatal(err)
			}
			artifact := filepath.Join(target, "sentinel")
			if err := os.WriteFile(artifact, []byte("retain target"), 0o600); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(home, "adopted-cache")
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			settings := storage.DefaultSettings()
			settings.GoCache.Enabled = true
			settings.GoCache.MaxBytes = 1
			settings.GoCache.AdoptedPath = link
			store := &recordingStore{}
			provider := New(Config{
				HomeDir: home, TrashDir: filepath.Join(home, "trash"),
				Settings: staticSettings{settings: settings}, Store: store,
			})

			var err error
			if explicit {
				_, err = provider.CleanupExplicit(context.Background())
			} else {
				_, err = provider.Cleanup(context.Background())
			}
			if err == nil {
				t.Fatal("cleanup succeeded through an adopted root symlink")
			}
			if store.created != nil || len(store.entries) != 0 {
				t.Fatalf("cleanup persisted quarantine state: created=%#v entries=%#v", store.created, store.entries)
			}
			if data, readErr := os.ReadFile(artifact); readErr != nil || string(data) != "retain target" {
				t.Fatalf("target data changed: data=%q err=%v", data, readErr)
			}
			if info, statErr := os.Lstat(link); statErr != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("adopted link changed: info=%v err=%v", info, statErr)
			}
		})
	}
}

func TestValidateAdoptionRejectsSymlinks(t *testing.T) {
	for _, test := range []struct {
		name string
		path func(t *testing.T, root, target string) string
	}{
		{
			name: "root",
			path: func(t *testing.T, root, target string) string {
				link := filepath.Join(root, "cache-link")
				if err := os.Symlink(target, link); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				return link
			},
		},
		{
			name: "ancestor",
			path: func(t *testing.T, root, target string) string {
				link := filepath.Join(root, "external-link")
				if err := os.Symlink(filepath.Dir(target), link); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				return filepath.Join(link, filepath.Base(target))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(t.TempDir(), "go-build")
			if err := os.MkdirAll(target, 0o700); err != nil {
				t.Fatal(err)
			}
			artifact := filepath.Join(target, "sentinel")
			if err := os.WriteFile(artifact, []byte("retain target"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := test.path(t, root, target)
			provider := New(Config{HomeDir: filepath.Join(root, "home"), TrashDir: filepath.Join(root, "trash")})

			err := provider.ValidateAdoption(context.Background(), path, "ADOPT")
			if err == nil {
				t.Fatal("ValidateAdoption succeeded through a symlink")
			}
			if errors.Is(err, ErrAdoptionConfirmation) {
				t.Fatalf("ValidateAdoption returned confirmation error despite valid confirmation: %v", err)
			}
			if data, readErr := os.ReadFile(artifact); readErr != nil || string(data) != "retain target" {
				t.Fatalf("target data changed: data=%q err=%v", data, readErr)
			}
		})
	}
}
