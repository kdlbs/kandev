package agents

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestDevinACPRemoteAuth(t *testing.T) {
	auth := NewDevinACP().RemoteAuth()
	if auth == nil {
		t.Fatal("RemoteAuth() returned nil; expected files and env auth methods")
	}
	if len(auth.Methods) != 2 {
		t.Fatalf("Methods len = %d, want 2", len(auth.Methods))
	}

	files := auth.Methods[0]
	if files.Type != "files" {
		t.Errorf("first Type = %q, want files", files.Type)
	}
	if files.Label == "" {
		t.Error("files auth method should have a UI label")
	}
	if files.TargetRelDir != devinCredentialsDir {
		t.Errorf("TargetRelDir = %q, want %q", files.TargetRelDir, devinCredentialsDir)
	}
	for _, goos := range []string{"darwin", "linux"} {
		got := files.SourceFiles[goos]
		want := []string{devinCredentialsRelPath}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("SourceFiles[%q] = %v, want %v", goos, got, want)
		}
	}

	env := auth.Methods[1]
	if env.Type != "env" {
		t.Errorf("second Type = %q, want env", env.Type)
	}
	if env.EnvVar != "WINDSURF_API_KEY" {
		t.Errorf("EnvVar = %q, want WINDSURF_API_KEY", env.EnvVar)
	}
	for _, needle := range []string{
		"credentials.toml",
		"windsurf_api_key",
		"api_server_url",
		devinDefaultAPIServer,
		"chmod 600",
	} {
		if !strings.Contains(env.SetupScript, needle) {
			t.Errorf("SetupScript missing %q: %q", needle, env.SetupScript)
		}
	}
}

func TestDevinACPInstallScriptUsesOfficialInstaller(t *testing.T) {
	script := NewDevinACP().InstallScript()
	for _, needle := range []string{
		"curl -fsSL https://cli.devin.ai/install.sh",
		`trap 'rm -f "$tmp"' EXIT`,
		"$HOME/.local/bin/devin",
		`export PATH="$HOME/.local/bin:$PATH"`,
		"persist_devin_path",
		"$HOME/.profile",
		"$HOME/.zprofile",
		"devin --version",
	} {
		if !strings.Contains(script, needle) {
			t.Errorf("InstallScript missing %q: %q", needle, script)
		}
	}
	if strings.Contains(script, "Install Devin CLI from") {
		t.Errorf("InstallScript must be executable shell, got prose: %q", script)
	}
	if strings.HasPrefix(script, "npm install -g ") {
		t.Errorf("InstallScript should use the native Devin installer, got npm script: %q", script)
	}
}

func TestDevinACPInstallScriptToleratesPostInstallSetupFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install script is POSIX shell")
	}

	home := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}

	fakeCurl := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    out="$1"
  fi
  shift
done
cat > "$out" <<'INSTALL'
#!/bin/sh
mkdir -p "$HOME/.local/bin"
cat > "$HOME/.local/bin/devin" <<'DEVIN'
#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "devin test"
  exit 0
fi
exit 0
DEVIN
chmod +x "$HOME/.local/bin/devin"
exit 1
INSTALL
`
	fakeCurlPath := filepath.Join(binDir, "curl")
	if err := os.WriteFile(fakeCurlPath, []byte(fakeCurl), 0o755); err != nil {
		t.Fatalf("write fake curl: %v", err)
	}

	cmd := exec.Command("sh", "-c", NewDevinACP().InstallScript())
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + binDir + ":/usr/bin:/bin",
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("InstallScript failed: %v\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(home, ".local/bin/devin")); err != nil {
		t.Fatalf("expected devin binary to be installed: %v", err)
	}
	for _, rel := range []string{".profile", ".bash_profile", ".bashrc", ".zprofile", ".zshrc"} {
		data, err := os.ReadFile(filepath.Join(home, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.Contains(string(data), `export PATH="$HOME/.local/bin:$PATH"`) {
			t.Fatalf("%s missing PATH export: %q", rel, string(data))
		}
	}
}
