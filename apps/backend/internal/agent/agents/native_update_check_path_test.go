package agents

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
)

// resolvePathAgainst mirrors Node's path.resolve(cwd, entry) used by OMP for
// PI_CONFIG_FILES and npm_config_userconfig, including Windows root-relative
// and drive-relative forms.
func TestResolvePathAgainstWindowsSemantics(t *testing.T) {
	cwd := `D:\work\repo`
	for _, tc := range []struct{ entry, want string }{
		{`C:\abs\channel.yml`, `C:\abs\channel.yml`},
		{`C:/abs/channel.yml`, `C:/abs/channel.yml`},
		{`\\server\share\channel.yml`, `\\server\share\channel.yml`},
		{`\omp-config\channel.yml`, `D:\omp-config\channel.yml`},
		{`/omp-config/channel.yml`, `D:/omp-config/channel.yml`},
		{`D:rel\channel.yml`, `D:\work\repo\rel\channel.yml`},
		{`E:rel\channel.yml`, `E:\rel\channel.yml`},
		{`rel\channel.yml`, `D:\work\repo\rel\channel.yml`},
	} {
		if got := resolvePathAgainst(cwd, tc.entry, true); got != tc.want {
			t.Errorf("resolvePathAgainst(%q) = %q, want %q", tc.entry, got, tc.want)
		}
	}
}

// A root-relative entry keeps the UNC server and share of a UNC working
// directory, as Node path.win32.resolve does.
func TestResolvePathAgainstWindowsUNCWorkingDir(t *testing.T) {
	cwd := `\\server\share\work\repo`
	for _, tc := range []struct{ entry, want string }{
		{`\config\channel.yml`, `\\server\share\config\channel.yml`},
		{`rel\channel.yml`, `\\server\share\work\repo\rel\channel.yml`},
	} {
		if got := resolvePathAgainst(cwd, tc.entry, true); got != tc.want {
			t.Errorf("resolvePathAgainst(%q) = %q, want %q", tc.entry, got, tc.want)
		}
	}
}

// OMP reads npm_config_userconfig with fs.readFile, so the filesystem follows
// a symlink before "..". The fingerprint must stat that same file.
func TestOmpFingerprintFollowsNpmUserconfigThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX symlink traversal semantics")
	}
	f := newOmpContextFixture(t)
	resolved := filepath.Join(f.root, "resolved")
	if err := os.MkdirAll(filepath.Join(resolved, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(f.cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(resolved, "child"), filepath.Join(f.cwd, "link")); err != nil {
		t.Fatal(err)
	}
	actual := filepath.Join(resolved, "npmrc")
	writeTestFile(t, actual, "registry=https://a.invalid/\n")
	declared := NewOmpACP().NativeUpdateCheck().Context
	resolver := NativeUpdateCheckResolver{
		Env: []string{"npm_config_userconfig=link/../npmrc"}, WorkingDir: f.cwd, HomeDir: f.home,
	}
	before := declared.Fingerprint(resolver)
	writeTestFile(t, actual, "registry=https://a-longer-registry.invalid/\n")
	if declared.Fingerprint(resolver) == before {
		t.Error("editing the npmrc OMP actually reads did not change the fingerprint")
	}
}

// OMP takes its configuration root from os.homedir(), which falls back to the
// effective account's home when HOME is unset or empty. The resolver must
// use that home rather than a working-directory-relative path.
func TestNativeUpdateCheckResolverFallsBackToAccountHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX account home lookup")
	}
	account, err := user.Current()
	if err != nil || account.HomeDir == "" {
		t.Skip("no account home available")
	}
	t.Setenv("HOME", "")
	for _, env := range [][]string{nil, {"HOME="}} {
		if got := NewNativeUpdateCheckResolver(env, t.TempDir(), "").HomeDir; got != account.HomeDir {
			t.Errorf("env %q: HomeDir = %q, want account home %q", env, got, account.HomeDir)
		}
	}
}
