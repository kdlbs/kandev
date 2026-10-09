package agents

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// Identity, every command surface, detection, logos, and the session-dir
// template are pinned by the shared newACPAgentSpecs matrix; the managed npm
// package contract by TestManagedNPMRuntimeContracts. These cover what is
// Muse-specific: the native-binary install, credentials, and resume paths.

func TestMuseACP_InstallScriptUsesOfficialInstallerInFirstWritablePathDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install script is POSIX shell")
	}
	script := NewMuseACP().InstallScript()
	for _, needle := range []string{
		"curl -fsSL https://dev.meta.ai/install.sh",
		`MUSE_INSTALL_DIR="$install_dir" bash "$tmp"`,
	} {
		if !strings.Contains(script, needle) {
			t.Errorf("InstallScript missing %q", needle)
		}
	}

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
set -eu
mkdir -p "$MUSE_INSTALL_DIR"
cat > "$MUSE_INSTALL_DIR/muse" <<'MUSE'
#!/bin/sh
echo "muse 1.2.1"
MUSE
chmod +x "$MUSE_INSTALL_DIR/muse"
INSTALL
`
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatalf("write fake curl: %v", err)
	}

	cmd := exec.Command("sh", "-c", script)
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + binDir + ":/usr/bin:/bin"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("InstallScript failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(binDir, "muse")); err != nil {
		t.Fatalf("expected muse binary in first writable PATH directory: %v", err)
	}
}

func TestMuseACP_InstallScriptPropagatesDownloadFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install script is POSIX shell")
	}
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake curl: %v", err)
	}
	cmd := exec.Command("sh", "-c", NewMuseACP().InstallScript())
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + binDir + ":/usr/bin:/bin"}
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("InstallScript succeeded after curl failure; output: %s", out)
	}
}

func TestMuseACP_RemoteAuth(t *testing.T) {
	auth := NewMuseACP().RemoteAuth()
	if auth == nil || len(auth.Methods) != 2 {
		t.Fatalf("RemoteAuth() = %+v, want files + env methods", auth)
	}

	files := auth.Methods[0]
	if files.Type != remoteAuthMethodTypeFiles || files.TargetRelDir != ".config/muse" {
		t.Errorf("Methods[0] = %+v, want files into .config/muse", files)
	}
	for _, osName := range []string{"darwin", "linux"} {
		if got, want := files.SourceFiles[osName], []string{".config/muse/auth.json"}; !slices.Equal(got, want) {
			t.Errorf("SourceFiles[%s] = %#v, want %#v", osName, got, want)
		}
	}

	if env := auth.Methods[1]; env.Type != "env" || env.EnvVar != "META_API_KEY" {
		t.Errorf("Methods[1] = %+v, want env META_API_KEY", env)
	}
}

func TestMuseACP_LoginCommand(t *testing.T) {
	cmd := NewMuseACP().LoginCommand()
	if cmd == nil || !slices.Equal(cmd.Cmd, []string{"env", "-u", "XDG_CONFIG_HOME", "-u", "XDG_DATA_HOME", "muse", "login"}) {
		t.Fatalf("LoginCommand() = %+v, want Muse login with XDG overrides removed", cmd)
	}
}

func TestMuseACP_PassthroughModelFlag(t *testing.T) {
	flag := NewMuseACP().PassthroughConfig().ModelFlag
	if got, want := flag.Args(), []string{"--model", "{model}"}; !slices.Equal(got, want) {
		t.Errorf("ModelFlag = %#v, want %#v", got, want)
	}
}

func TestMuseACP_SessionConfigAndMCPDelivery(t *testing.T) {
	rt := NewMuseACP().Runtime()
	sc := rt.SessionConfig
	if !sc.NativeSessionResume || sc.CanRecover == nil || !*sc.CanRecover {
		t.Errorf("SessionConfig = %+v, want native resume and recovery", sc)
	}
	if sc.SessionDirTarget != "/root" {
		t.Errorf("SessionDirTarget = %q, want /root", sc.SessionDirTarget)
	}
	if !slices.Equal(rt.StripEnv, []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME"}) {
		t.Errorf("Runtime.StripEnv = %v, want XDG overrides removed", rt.StripEnv)
	}
	// The adapter forwards session/new MCP servers itself; a project file
	// strategy would duplicate them.
	if rt.ProjectMCPStrategy != nil {
		t.Errorf("ProjectMCPStrategy = %#v, want nil", rt.ProjectMCPStrategy)
	}
}
