package sshconnections

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const enrollmentTestKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBJ1cZIiNeIWglinQZGj3KoEKWIqsRnCEpbcbUUG+j9A test"

func runEnrollmentScript(t *testing.T, home string) error {
	t.Helper()
	cmd := exec.Command("sh", "-c", enrollmentScript)
	cmd.Env = append(os.Environ(), "HOME="+home)
	cmd.Stdin = strings.NewReader(enrollmentTestKey + "\n")
	return cmd.Run()
}
func TestEnrollmentScriptPreservesAndAppendsIdempotently(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "authorized_keys")
	existing := "# existing entry without trailing newline"
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runEnrollmentScript(t, home); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != existing+"\n"+enrollmentTestKey+"\n" {
		t.Fatalf("unexpected append: %q", first)
	}
	if err = runEnrollmentScript(t, home); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("retry changed authorized_keys")
	}
	restricted := "restrict,command=\"echo restricted\" " + enrollmentTestKey + "\n"
	if err = os.WriteFile(path, []byte(restricted), 0600); err != nil {
		t.Fatal(err)
	}
	if err = runEnrollmentScript(t, home); err != nil {
		t.Fatal(err)
	}
	second, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != restricted {
		t.Fatal("appended an unrestricted duplicate")
	}
}
func TestEnrollmentScriptRejectsSymlinksAndUnsafePermissions(t *testing.T) {
	for _, scenario := range []string{"directory_symlink", "file_symlink", "unsafe_file", "locked"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".ssh")
			outside := filepath.Join(t.TempDir(), "untouched")
			if err := os.WriteFile(outside, []byte("sentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			setupUnsafeEnrollment(t, scenario, dir, outside)

			if err := runEnrollmentScript(t, home); err == nil {
				t.Fatal("unsafe enrollment succeeded")
			}
			data, err := os.ReadFile(outside)
			if err != nil || string(data) != "sentinel" {
				t.Fatal("modified symlink target")
			}
		})
	}
}

func setupUnsafeEnrollment(t *testing.T, scenario, dir, outside string) {
	t.Helper()
	if scenario == "directory_symlink" {
		if err := os.Symlink(filepath.Dir(outside), dir); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	switch scenario {
	case "file_symlink":
		if err := os.Symlink(outside, filepath.Join(dir, "authorized_keys")); err != nil {
			t.Fatal(err)
		}
	case "unsafe_file":
		path := filepath.Join(dir, "authorized_keys")
		if err := os.WriteFile(path, []byte("existing"), 0666); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0666); err != nil {
			t.Fatal(err)
		}
	case "locked":
		if err := os.Mkdir(filepath.Join(dir, ".kandev-enrollment-lock"), 0700); err != nil {
			t.Fatal(err)
		}
	}
}
