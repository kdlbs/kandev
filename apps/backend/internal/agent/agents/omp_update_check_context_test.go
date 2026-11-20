package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestOmpUpdateCheckContextDeclaresUpstreamEnvKeys(t *testing.T) {
	want := []string{
		"OMP_PROFILE", "PI_PROFILE", "PI_CODING_AGENT_DIR", "PI_CONFIG_DIR", "PI_CONFIG_FILES",
		"HOME", "XDG_CONFIG_HOME", "npm_config_userconfig", "npm_config_registry", "BUN_CONFIG_REGISTRY",
		"MISE_DATA_DIR", "XDG_DATA_HOME", "LOCALAPPDATA", "BUN_INSTALL_GLOBAL_DIR", "PI_BUILD_URL",
	}
	got := NewOmpACP().NativeUpdateCheck().Context.EnvKeys
	if !slices.Equal(got, want) {
		t.Errorf("EnvKeys = %v, want %v", got, want)
	}
}

// Upstream classifies the install method from MISE_DATA_DIR (with its
// LOCALAPPDATA/XDG_DATA_HOME fallbacks), BUN_INSTALL_GLOBAL_DIR, and the
// launcher's `<name>.bunx` marker; each selects a different release source.
func TestOmpUpdateCheckContextFingerprintTracksInstallMethodSelectors(t *testing.T) {
	f := newOmpContextFixture(t)
	declared := NewOmpACP().NativeUpdateCheck().Context
	launcher := filepath.Join(f.root, "shims", "omp")
	resolver := NativeUpdateCheckResolver{WorkingDir: f.cwd, HomeDir: f.home, Executable: f.exe, Launcher: launcher}
	base := declared.Fingerprint(resolver)
	for _, entry := range []string{
		"MISE_DATA_DIR=" + f.root, "XDG_DATA_HOME=" + f.root, "LOCALAPPDATA=" + f.root, "BUN_INSTALL_GLOBAL_DIR=" + f.root,
	} {
		changed := resolver
		changed.Env = []string{entry}
		if declared.Fingerprint(changed) == base {
			t.Errorf("%s did not change the fingerprint", entry)
		}
	}
	writeTestFile(t, filepath.Join(f.root, "shims", "omp.bunx"), "marker")
	if declared.Fingerprint(resolver) == base {
		t.Error("creating the launcher's .bunx ownership marker did not change the fingerprint")
	}
}

type ompContextFixture struct {
	root, home, cwd, exe string
}

func newOmpContextFixture(t *testing.T) ompContextFixture {
	root := t.TempDir()
	return ompContextFixture{
		root: root,
		home: filepath.Join(root, "home"),
		cwd:  filepath.Join(root, "work"),
		exe:  filepath.Join(root, "install", "bin", "omp"),
	}
}

func (f ompContextFixture) sources(env ...string) NativeUpdateCheckSources {
	resolver := NativeUpdateCheckResolver{Env: env, WorkingDir: f.cwd, HomeDir: f.home, Executable: f.exe}
	return NewOmpACP().NativeUpdateCheck().Context.Sources(resolver)
}

func (f ompContextFixture) defaultFiles(agentDir, root string) []string {
	base := filepath.Join(f.home, ".omp")
	return []string{
		filepath.Join(agentDir, "config.yml"), filepath.Join(agentDir, "config.yaml"),
		filepath.Join(f.cwd, ".omp", "config.yml"), filepath.Join(f.cwd, ".omp", "settings.json"),
		filepath.Join(f.cwd, ".claude", "settings.json"), filepath.Join(f.cwd, ".claude", "settings.yml"),
		filepath.Join(f.root, "install", "manager.json"),
		filepath.Join(f.home, ".npmrc"), filepath.Join(f.home, ".bunfig.toml"),
		filepath.Join(f.cwd, ".env"), filepath.Join(f.home, ".env"), filepath.Join(root, ".env"), filepath.Join(base, ".env"), filepath.Join(agentDir, ".env"),
	}
}

func assertSourceFiles(t *testing.T, got NativeUpdateCheckSources, want []string) {
	t.Helper()
	gotFiles := slices.Clone(got.Files)
	slices.Sort(gotFiles)
	gotFiles = slices.Compact(gotFiles)
	wantFiles := slices.Clone(want)
	slices.Sort(wantFiles)
	wantFiles = slices.Compact(wantFiles)
	if !slices.Equal(gotFiles, wantFiles) {
		t.Errorf("files =\n%s\nwant\n%s", strings.Join(gotFiles, "\n"), strings.Join(wantFiles, "\n"))
	}
}

func TestOmpUpdateCheckContextProfiles(t *testing.T) {
	f := newOmpContextFixture(t)
	base := filepath.Join(f.home, ".omp")
	work := filepath.Join(base, "profiles", "work")
	workAgent := filepath.Join(work, "agent")
	custom := filepath.Join(f.root, "custom-agent")
	cases := []struct {
		name     string
		env      []string
		agentDir string
		root     string
	}{
		{name: "default profile", agentDir: filepath.Join(base, "agent"), root: base},
		{name: "explicit default profile", env: []string{"OMP_PROFILE=default"}, agentDir: filepath.Join(base, "agent"), root: base},
		{name: "agent dir override", env: []string{"PI_CODING_AGENT_DIR=" + custom}, agentDir: custom, root: base},
		{name: "named OMP profile ignores agent dir override", env: []string{"OMP_PROFILE=work", "PI_CODING_AGENT_DIR=" + custom}, agentDir: workAgent, root: work},
		{name: "legacy PI profile", env: []string{"PI_PROFILE=work"}, agentDir: workAgent, root: work},
		{name: "OMP profile wins over PI profile", env: []string{"OMP_PROFILE=work", "PI_PROFILE=home"}, agentDir: workAgent, root: work},
		{name: "trimmed profile", env: []string{"OMP_PROFILE= work "}, agentDir: workAgent, root: work},
		{name: "explicitly empty OMP profile selects default", env: []string{"OMP_PROFILE=", "PI_PROFILE=work"}, agentDir: filepath.Join(base, "agent"), root: base},
		{
			name:     "empty OMP profile ignores inherited profile-derived agent dir",
			env:      []string{"OMP_PROFILE=", "PI_PROFILE=work", "PI_CODING_AGENT_DIR=" + workAgent},
			agentDir: filepath.Join(base, "agent"), root: base,
		},
		{
			name:     "default OMP profile ignores inherited profile-derived agent dir",
			env:      []string{"OMP_PROFILE=default", "PI_PROFILE=work", "PI_CODING_AGENT_DIR=" + workAgent},
			agentDir: filepath.Join(base, "agent"), root: base,
		},
		{
			name:     "agent dir for another profile is an override",
			env:      []string{"OMP_PROFILE=", "PI_PROFILE=home", "PI_CODING_AGENT_DIR=" + workAgent},
			agentDir: workAgent, root: base,
		},
		{name: "invalid profile falls back to default", env: []string{"OMP_PROFILE=Bad Name"}, agentDir: filepath.Join(base, "agent"), root: base},
		{name: "reserved profile falls back to default", env: []string{"OMP_PROFILE=con.txt"}, agentDir: filepath.Join(base, "agent"), root: base},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertSourceFiles(t, f.sources(tc.env...), f.defaultFiles(tc.agentDir, tc.root))
		})
	}
}

func TestOmpUpdateCheckContextConfigDirAndOverlays(t *testing.T) {
	f := newOmpContextFixture(t)
	t.Run("config dir name", func(t *testing.T) {
		base := filepath.Join(f.home, ".pi")
		got := f.sources("PI_CONFIG_DIR=.pi")
		assertSourceFiles(t, got, f.defaultFilesWithBase(base, filepath.Join(base, "agent"), base))
	})
	t.Run("overlay files", func(t *testing.T) {
		abs := filepath.Join(f.root, "shared", "team.yml")
		list := strings.Join([]string{"~/overlay.yml", "", filepath.Join("rel", "local.yml"), abs}, string(os.PathListSeparator))
		got := f.sources("PI_CONFIG_FILES=" + list)
		want := append(f.defaultFiles(filepath.Join(f.home, ".omp", "agent"), filepath.Join(f.home, ".omp")),
			filepath.Join(f.home, "overlay.yml"), filepath.Join(f.cwd, "rel", "local.yml"), abs)
		assertSourceFiles(t, got, want)
	})
	t.Run("overlay paths keep whitespace and expand bare tilde like upstream expandTilde", func(t *testing.T) {
		list := strings.Join([]string{"overlay.yml ", "~team.yml"}, string(os.PathListSeparator))
		got := f.sources("PI_CONFIG_FILES=" + list)
		want := append(f.defaultFiles(filepath.Join(f.home, ".omp", "agent"), filepath.Join(f.home, ".omp")),
			filepath.Join(f.cwd, "overlay.yml "), filepath.Join(f.home, "team.yml"))
		assertSourceFiles(t, got, want)
	})
}

// Windows environment lookup is case-insensitive, so a mixed-case declared key
// still selects OMP's configuration; an empty value still masks a fallback.
func TestOmpUpdateCheckContextFoldsKeyCaseOnWindows(t *testing.T) {
	f := newOmpContextFixture(t)
	overlay := filepath.Join(f.root, "channel.yml")
	resolver := NativeUpdateCheckResolver{Env: []string{"pi_config_files=" + overlay}, WorkingDir: f.cwd, HomeDir: f.home, Executable: f.exe, FoldCase: true}
	got := NewOmpACP().NativeUpdateCheck().Context.Sources(resolver)
	assertSourceFiles(t, got, append(f.defaultFiles(filepath.Join(f.home, ".omp", "agent"), filepath.Join(f.home, ".omp")), overlay))
	resolver.FoldCase = false
	if got := NewOmpACP().NativeUpdateCheck().Context.Sources(resolver); slices.Contains(got.Files, overlay) {
		t.Errorf("POSIX lookup matched a differently cased key: %v", got.Files)
	}
	masked := NativeUpdateCheckResolver{Env: []string{"omp_profile=", "PI_PROFILE=work"}, FoldCase: true}
	if value, ok := masked.Getenv("OMP_PROFILE"); !ok || value != "" {
		t.Errorf("Getenv(OMP_PROFILE) = %q, %v; want a present empty value", value, ok)
	}
}

// Through the production constructor, a lowercase PI_CONFIG_FILES selects the
// overlay on Windows, where environment keys are case-insensitive, and not on
// POSIX. Editing the overlay must change the Windows fingerprint.
func TestNativeUpdateCheckResolverPlatformCase(t *testing.T) {
	dir := t.TempDir()
	overlay := filepath.Join(dir, "channel.yml")
	if err := os.WriteFile(overlay, []byte("update:\n  channel: stable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	context := NewOmpACP().NativeUpdateCheck().Context
	resolver := NewNativeUpdateCheckResolver([]string{"pi_config_files=" + overlay}, dir, "")
	before := context.Fingerprint(resolver)
	if err := os.WriteFile(overlay, []byte("update:\n  channel: canary-longer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed := context.Fingerprint(resolver) != before
	if want := runtime.GOOS == "windows"; changed != want {
		t.Errorf("on %s, editing a lowercase-keyed overlay changed the fingerprint = %v, want %v", runtime.GOOS, changed, want)
	}
}

func (f ompContextFixture) defaultFilesWithBase(base, agentDir, root string) []string {
	files := f.defaultFiles(agentDir, root)
	for i, path := range files {
		if path == filepath.Join(f.home, ".omp", ".env") {
			files[i] = filepath.Join(base, ".env")
		}
	}
	return files
}

func TestOmpUpdateCheckContextReleaseFeedAndOwnership(t *testing.T) {
	f := newOmpContextFixture(t)
	base := filepath.Join(f.home, ".omp")
	defaults := f.defaultFiles(filepath.Join(base, "agent"), base)
	replace := func(files []string, from, to string) []string {
		out := slices.Clone(files)
		for i := range out {
			if out[i] == from {
				out[i] = to
			}
		}
		return out
	}
	userconfig := filepath.Join(f.root, "npm", "userrc")
	xdg := filepath.Join(f.root, "xdg")
	t.Run("npm userconfig", func(t *testing.T) {
		assertSourceFiles(t, f.sources("npm_config_userconfig="+userconfig), replace(defaults, filepath.Join(f.home, ".npmrc"), userconfig))
	})
	t.Run("npm userconfig matched case-insensitively", func(t *testing.T) {
		assertSourceFiles(t, f.sources("NPM_CONFIG_USERCONFIG="+userconfig), replace(defaults, filepath.Join(f.home, ".npmrc"), userconfig))
	})
	t.Run("exact npm key wins", func(t *testing.T) {
		other := filepath.Join(f.root, "npm", "upper")
		assertSourceFiles(t, f.sources("NPM_CONFIG_USERCONFIG="+other, "npm_config_userconfig="+userconfig), replace(defaults, filepath.Join(f.home, ".npmrc"), userconfig))
	})
	t.Run("xdg bunfig", func(t *testing.T) {
		assertSourceFiles(t, f.sources("XDG_CONFIG_HOME="+xdg), append(slices.Clone(defaults), filepath.Join(xdg, ".bunfig.toml")))
	})
	t.Run("unresolved executable has no ownership manifest", func(t *testing.T) {
		resolver := NativeUpdateCheckResolver{WorkingDir: f.cwd, HomeDir: f.home}
		got := NewOmpACP().NativeUpdateCheck().Context.Sources(resolver)
		want := slices.DeleteFunc(slices.Clone(defaults), func(path string) bool { return strings.HasSuffix(path, "manager.json") })
		assertSourceFiles(t, got, want)
	})
}

func TestOmpUpdateCheckContextProjectDirectories(t *testing.T) {
	f := newOmpContextFixture(t)
	dirs := f.sources().Dirs
	byPath := map[string]NativeUpdateCheckDir{}
	for _, dir := range dirs {
		byPath[dir.Path] = dir
	}
	if len(byPath) != 3 {
		t.Fatalf("dirs = %+v, want .omp, .claude and cwd dotenv", dirs)
	}
	for _, name := range []string{".omp", ".claude"} {
		dir, ok := byPath[filepath.Join(f.cwd, name)]
		if !ok || dir.Match != nil {
			t.Errorf("%s directory = %+v, want every regular file", name, dir)
		}
	}
	dotenv, ok := byPath[f.cwd]
	if !ok || dotenv.Match == nil {
		t.Fatalf("cwd dotenv directory missing a name filter")
	}
	for name, want := range map[string]bool{".env": true, ".env.local": true, ".env.production.local": true, ".envrc": false, "app.env": false, "env": false} {
		if got := dotenv.Match(name); got != want {
			t.Errorf("dotenv match %q = %v, want %v", name, got, want)
		}
	}
}

func TestOmpUpdateCheckContextExecutableSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "tern", "versions", "18.13.3", "omp")
	writeTestFile(t, target, "binary")
	link := filepath.Join(root, "bin", "omp")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	home := filepath.Join(root, "home")
	resolver := NewNativeUpdateCheckResolver([]string{"HOME=" + home, "USERPROFILE=" + home}, root, link)
	wantExe, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if resolver.Executable != wantExe || resolver.HomeDir != home || resolver.WorkingDir != root {
		t.Fatalf("resolver = %+v, want executable %s", resolver, wantExe)
	}
	files := NewOmpACP().NativeUpdateCheck().Context.Sources(resolver).Files
	manifest := filepath.Join(filepath.Dir(filepath.Dir(wantExe)), "manager.json")
	if !slices.Contains(files, manifest) {
		t.Errorf("files %v do not contain ownership manifest %s", files, manifest)
	}
}

func TestOmpUpdateCheckContextFingerprintTracksDeclaredInputs(t *testing.T) {
	f := newOmpContextFixture(t)
	declared := NewOmpACP().NativeUpdateCheck().Context
	writeTestFile(t, filepath.Join(f.cwd, ".omp", "config.yml"), "a")
	resolver := NativeUpdateCheckResolver{Env: []string{"PI_PROFILE=work", "UNRELATED=1"}, WorkingDir: f.cwd, HomeDir: f.home, Executable: f.exe}
	base := declared.Fingerprint(resolver)
	if base == "" || declared.Fingerprint(resolver) != base {
		t.Fatal("fingerprint is not stable")
	}
	changedEnv := resolver
	changedEnv.Env = []string{"PI_PROFILE=home", "UNRELATED=1"}
	if declared.Fingerprint(changedEnv) == base {
		t.Error("declared env value change kept the fingerprint")
	}
	unrelated := resolver
	unrelated.Env = []string{"PI_PROFILE=work", "UNRELATED=2"}
	if declared.Fingerprint(unrelated) != base {
		t.Error("undeclared env value changed the fingerprint")
	}
	foldedKey := resolver
	foldedKey.Env = []string{"PI_PROFILE=work", "UNRELATED=1", "NPM_CONFIG_REGISTRY=https://example.invalid"}
	if declared.Fingerprint(foldedKey) == base {
		t.Error("case-folded npm config key did not change the fingerprint")
	}
	extra := filepath.Join(f.cwd, ".omp", "extensions.json")
	writeTestFile(t, extra, "x")
	withExtra := declared.Fingerprint(resolver)
	if withExtra == base {
		t.Error("new project file did not change the fingerprint")
	}
	writeTestFile(t, extra, "longer")
	if declared.Fingerprint(resolver) == withExtra {
		t.Error("project file size change kept the fingerprint")
	}
}

func TestOmpUpdateCheckFingerprintTracksWorkingDirectoryEnvBeyondScanLimit(t *testing.T) {
	f := newOmpContextFixture(t)
	if err := os.MkdirAll(f.cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := range nativeUpdateCheckDirEntryLimit + 1 {
		path := filepath.Join(f.cwd, fmt.Sprintf("unrelated-%03d", i))
		writeTestFile(t, path, "ordinary file")
	}
	envPath := filepath.Join(f.cwd, ".env")
	writeTestFile(t, envPath, "A")
	context := NewOmpACP().NativeUpdateCheck().Context
	resolver := NativeUpdateCheckResolver{WorkingDir: f.cwd, HomeDir: f.home, Executable: f.exe}
	before := context.Fingerprint(resolver)
	writeTestFile(t, envPath, "a longer environment value")
	if context.Fingerprint(resolver) == before {
		t.Error("working-directory .env edit was hidden by the directory scan limit")
	}
}

func TestOmpUpdateCheckContextDirectoryEntryLimit(t *testing.T) {
	f := newOmpContextFixture(t)
	dir := filepath.Join(f.cwd, ".omp")
	known := filepath.Join(dir, "config.yml")
	writeTestFile(t, known, "a")
	for i := range nativeUpdateCheckDirEntryLimit {
		writeTestFile(t, filepath.Join(dir, fmt.Sprintf("extra-%03d", i)), "x")
	}
	declared := NewOmpACP().NativeUpdateCheck().Context
	resolver := NativeUpdateCheckResolver{WorkingDir: f.cwd, HomeDir: f.home}
	pinDirTime := func() {
		t.Helper()
		stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := os.Chtimes(dir, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	pinDirTime()
	base := declared.Fingerprint(resolver)
	sample := filepath.Join(dir, "extra-000")
	writeTestFile(t, sample, "edited without membership change")
	pinDirTime()
	if declared.Fingerprint(resolver) != base {
		t.Error("over the entry limit, a non-candidate file edit changed the fingerprint")
	}
	writeTestFile(t, known, "known candidate edited")
	pinDirTime()
	edited := declared.Fingerprint(resolver)
	if edited == base {
		t.Error("known candidate edit inside an over-limit directory kept the fingerprint")
	}
	writeTestFile(t, filepath.Join(dir, "new-file"), "x")
	if declared.Fingerprint(resolver) == edited {
		t.Error("creating a file in an over-limit directory kept the fingerprint")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
