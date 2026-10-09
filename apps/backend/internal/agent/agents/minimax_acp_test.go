package agents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMiniMaxDiscoveryChecksExecutableWithoutAuthentication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	bin := t.TempDir()
	file := filepath.Join(bin, "mcode")
	for _, tc := range []struct {
		name, body string
		available  bool
	}{
		{"working CLI", "[ \"$1\" = --version ] && exit 0\nexit 1\n", true},
		{"broken CLI", "exit 1\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(file, []byte("#!/bin/sh\n"+tc.body), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			result, err := NewMiniMaxACP().IsInstalled(context.Background())
			if err != nil || result.Available != tc.available {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestMiniMaxLoginUsesDefaultDataDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("login terminal requires POSIX shell")
	}
	bin := t.TempDir()
	file := filepath.Join(bin, "mcode")
	script := `#!/bin/sh
[ "$1" = login ] && [ "$2" = --no-browser ] || exit 1
[ "${MINIMAX_DATA_DIR+set}" != set ] || exit 2
[ "${MAVIS_DATA_DIR+set}" != set ] || exit 3
[ "${__MAVIS_RUNTIME_DATA_DIR+set}" != set ] || exit 4
[ "${__MAVIS_RUNTIME_PROFILE+set}" != set ] || exit 5
`
	if err := os.WriteFile(file, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, key := range minimaxDataOverrides() {
		t.Setenv(key, "outside-executor")
	}
	login := NewMiniMaxACP().LoginCommand()
	commands := append([][]string{login.Cmd}, login.Variants["cn"], login.Variants["global"])
	for _, argv := range commands {
		if out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("login failed: %v %s", err, out)
		}
	}
}

func TestMiniMaxInstallUsesOfficialPackageAndSQLiteScripts(t *testing.T) {
	script := NewMiniMaxACP().InstallScript()
	for _, part := range []string{"npm install -g @minimax-ai/code@0.5.10", "--registry=https://registry.npmjs.org/", "--ignore-scripts=false", "--include=optional", "--allow-scripts=@minimax-ai/code,better-sqlite3"} {
		if !strings.Contains(script, part) {
			t.Errorf("missing install setting %s", part)
		}
	}
}
