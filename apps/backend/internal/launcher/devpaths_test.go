package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	commonconfig "github.com/kandev/kandev/internal/common/config"
)

func makeRepoTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"apps/backend", "apps/web"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFindRepoRootFromNestedWorkingDirectory(t *testing.T) {
	root := makeRepoTree(t)

	got, err := findRepoRoot(filepath.Join(root, "apps", "backend", "internal", "launcher"))
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("findRepoRoot = %q, want %q", got, root)
	}
}

func TestFindRepoRootFromRepoRootItself(t *testing.T) {
	root := makeRepoTree(t)

	got, err := findRepoRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("findRepoRoot = %q, want %q", got, root)
	}
}

func TestFindRepoRootFromAppsDirectory(t *testing.T) {
	root := makeRepoTree(t)

	got, err := findRepoRoot(filepath.Join(root, "apps"))
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("findRepoRoot = %q, want %q", got, root)
	}
}

func TestFindRepoRootRequiresBothApps(t *testing.T) {
	onlyBackend := t.TempDir()
	if err := os.MkdirAll(filepath.Join(onlyBackend, "apps", "backend"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := findRepoRoot(onlyBackend); err == nil {
		t.Fatal("findRepoRoot accepted a tree with only apps/backend")
	}
}

func TestFindRepoRootFailsOutsideRepository(t *testing.T) {
	if _, err := findRepoRoot(t.TempDir()); err == nil {
		t.Fatal("findRepoRoot accepted an empty tree")
	}
}

func TestIsInsideKandevTaskSignals(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KANDEV_TASK_ID", "")

	repoInTasks := filepath.Join(home, ".kandev", "tasks", "ws", "repo")
	if !isInsideKandevTask(repoInTasks) {
		t.Fatalf("isInsideKandevTask(%q) = false, want true for a repo under ~/.kandev/tasks", repoInTasks)
	}

	if isInsideKandevTask(filepath.Join(home, "projects", "repo")) {
		t.Fatal("isInsideKandevTask = true for a repo outside the tasks dir")
	}

	tasksDir := filepath.Join(home, ".kandev", "tasks")
	if isInsideKandevTask(tasksDir) {
		t.Fatal("isInsideKandevTask = true for the tasks dir itself (no child segment)")
	}

	t.Setenv("KANDEV_TASK_ID", "task-123")
	if !isInsideKandevTask(filepath.Join(home, "projects", "repo")) {
		t.Fatal("isInsideKandevTask = false with KANDEV_TASK_ID set")
	}
}

func devEnvToMap(extra []string) map[string]string {
	out := map[string]string{}
	for _, item := range extra {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

func TestResolveDevBackendEnvDefaultUsesRepoLocalHome(t *testing.T) {
	repo := makeRepoTree(t)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "")

	dbPath, extra := resolveDevBackendEnv(repo)
	env := devEnvToMap(extra)
	wantHome := filepath.Join(repo, ".kandev-dev")
	wantDB := filepath.Join(wantHome, "data", "kandev.db")

	if dbPath != wantDB {
		t.Fatalf("dbPath = %q, want %q", dbPath, wantDB)
	}
	if env["KANDEV_HOME_DIR"] != wantHome {
		t.Fatalf("KANDEV_HOME_DIR = %q, want %q", env["KANDEV_HOME_DIR"], wantHome)
	}
	if env["KANDEV_DATABASE_PATH"] != wantDB {
		t.Fatalf("KANDEV_DATABASE_PATH = %q, want %q", env["KANDEV_DATABASE_PATH"], wantDB)
	}
	if env["KANDEV_DEBUG_DEV_MODE"] != "true" {
		t.Fatalf("KANDEV_DEBUG_DEV_MODE = %q, want true", env["KANDEV_DEBUG_DEV_MODE"])
	}
}

func TestResolveDevBackendEnvHonorsExplicitDatabasePath(t *testing.T) {
	repo := makeRepoTree(t)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "/custom/kandev.db")

	dbPath, extra := resolveDevBackendEnv(repo)
	env := devEnvToMap(extra)

	if dbPath != "/custom/kandev.db" {
		t.Fatalf("dbPath = %q, want the explicit override", dbPath)
	}
	if env["KANDEV_DATABASE_PATH"] != "/custom/kandev.db" {
		t.Fatalf("KANDEV_DATABASE_PATH = %q", env["KANDEV_DATABASE_PATH"])
	}
	if want := filepath.Join(repo, ".kandev-dev"); env["KANDEV_HOME_DIR"] != want {
		t.Fatalf("KANDEV_HOME_DIR = %q, want repo-local dev home %q", env["KANDEV_HOME_DIR"], want)
	}
	if env["KANDEV_DEBUG_DEV_MODE"] != "true" {
		t.Fatalf("KANDEV_DEBUG_DEV_MODE = %q, want true", env["KANDEV_DEBUG_DEV_MODE"])
	}
}

func TestResolveDevBackendEnvIgnoresWhitespaceOnlyOverride(t *testing.T) {
	repo := makeRepoTree(t)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "   ")

	dbPath, extra := resolveDevBackendEnv(repo)
	env := devEnvToMap(extra)
	wantHome := filepath.Join(repo, ".kandev-dev")

	if dbPath != filepath.Join(wantHome, "data", "kandev.db") {
		t.Fatalf("dbPath = %q, want the repo-local dev db for a blank override", dbPath)
	}
	if env["KANDEV_HOME_DIR"] != wantHome {
		t.Fatalf("KANDEV_HOME_DIR = %q, want %q", env["KANDEV_HOME_DIR"], wantHome)
	}
	if env["KANDEV_DATABASE_PATH"] != filepath.Join(wantHome, "data", "kandev.db") {
		t.Fatalf("KANDEV_DATABASE_PATH = %q, want repo-local database", env["KANDEV_DATABASE_PATH"])
	}
}

func TestResolveDevBackendEnvIgnoresAmbientConfiguredHome(t *testing.T) {
	repo := makeRepoTree(t)
	customHome := filepath.Join(t.TempDir(), "kandev")
	t.Setenv("KANDEV_HOME_DIR", customHome)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "")

	cfg := &commonconfig.Config{
		HomeDir: customHome,
		Source: commonconfig.ConfigSource{Values: map[string]commonconfig.SettingSource{
			"homeDir": commonconfig.SourceEnvironment,
		}},
	}
	dbPath, extra := resolveDevBackendEnv(repo, cfg)
	if want := filepath.Join(repo, ".kandev-dev", "data", "kandev.db"); dbPath != want {
		t.Fatalf("dbPath = %q, want %q", dbPath, want)
	}
	if env := devEnvToMap(extra); env["KANDEV_HOME_DIR"] != filepath.Join(repo, ".kandev-dev") {
		t.Fatalf("KANDEV_HOME_DIR = %q, want repo-local dev home", env["KANDEV_HOME_DIR"])
	}
}

func TestResolveDevBackendEnvHonorsConfiguredHomeDataDirectory(t *testing.T) {
	repo := makeRepoTree(t)
	t.Setenv("KANDEV_TASK_ID", "")
	configuredHome := filepath.Join(t.TempDir(), "configured-kandev")
	cfg := &commonconfig.Config{
		HomeDir: configuredHome,
		Source: commonconfig.ConfigSource{Values: map[string]commonconfig.SettingSource{
			"homeDir": commonconfig.SourceConfiguration,
		}},
	}

	dbPath, extra := resolveDevBackendEnv(repo, cfg)
	want := filepath.Join(configuredHome, "data", "kandev.db")
	if dbPath != want {
		t.Fatalf("dbPath = %q, want configured home data path %q", dbPath, want)
	}
	if got := devEnvToMap(extra)["KANDEV_DATABASE_PATH"]; got != want {
		t.Fatalf("KANDEV_DATABASE_PATH = %q, want %q", got, want)
	}
}

func TestDevLaunchConfigIgnoresAmbientHomeForSupervisorState(t *testing.T) {
	repo := makeRepoTree(t)
	t.Chdir(repo)
	t.Setenv("KANDEV_TASK_ID", "")
	ambientHome := filepath.Join(t.TempDir(), "kandev")
	cfg := &commonconfig.Config{
		HomeDir: ambientHome,
		Source: commonconfig.ConfigSource{Values: map[string]commonconfig.SettingSource{
			"homeDir": commonconfig.SourceEnvironment,
		}},
	}

	launch, code := devLaunchConfigFor(Options{Command: CommandDev}, cfg)
	if code != 0 {
		t.Fatalf("devLaunchConfigFor() = %d, want 0", code)
	}
	if want := filepath.Join(repo, ".kandev-dev"); launch.homeDir != want {
		t.Fatalf("supervisor home = %q, want repo-local %q", launch.homeDir, want)
	}
}

func TestResolveDevDatabaseTargetPreservesYamlHomeOverAmbientEnv(t *testing.T) {
	// GIVEN a working-directory YAML with an explicit homeDir AND an ambient
	// KANDEV_HOME_DIR that differs, WHEN the config is loaded through
	// devStartupConfig AND resolved through resolveDevDatabaseTarget,
	// THEN the YAML homeDir is preserved rather than silently discarded.
	repo := makeRepoTree(t)
	clearLauncherConfigurationEnvironment(t)
	t.Setenv("KANDEV_TASK_ID", "")
	configuredHome := filepath.Join(t.TempDir(), "configured-kandev")
	ambientHome := filepath.Join(t.TempDir(), "ambient-home")

	// Place a config.yaml in the working directory with an explicit homeDir.
	t.Chdir(repo)
	configPath := filepath.Join(repo, "config.yaml")
	writeLauncherConfig(t, configPath, "homeDir: "+configuredHome+"\n")

	// Set ambient KANDEV_HOME_DIR to a different value. Without the fix,
	// loadDevBootstrapConfig would pick up this env and the homeDir source
	// would become SourceEnvironment, causing devStateHome to fall back.
	t.Setenv("KANDEV_HOME_DIR", ambientHome)

	cfg, exitCode := devStartupConfig(repo)
	if exitCode != 0 {
		t.Fatalf("devStartupConfig() = %d, want 0", exitCode)
	}
	if cfg.SourceFor("homeDir") != commonconfig.SourceConfiguration {
		t.Fatalf("homeDir source = %q, want SourceConfiguration (YAML); ambient KANDEV_HOME_DIR must not override YAML during dev bootstrap", cfg.SourceFor("homeDir"))
	}
	if got := cfg.ResolvedHomeDir(); got != configuredHome {
		t.Fatalf("resolved homeDir = %q, want YAML value %q", got, configuredHome)
	}

	// Now resolve the database target with this config.
	target := resolveDevDatabaseTarget(repo, cfg)
	if target.homeDir != configuredHome {
		t.Fatalf("target homeDir = %q, want YAML-configured home %q", target.homeDir, configuredHome)
	}
	wantDB := filepath.Join(configuredHome, "data", "kandev.db")
	if target.path != wantDB {
		t.Fatalf("target database path = %q, want configured home data path %q", target.path, wantDB)
	}
	if target.source != devDatabaseConfigHome {
		t.Fatalf("target source = %q, want devDatabaseConfigHome", target.source)
	}
}

func TestResolveDevBackendEnvHonorsEnvironmentDatabaseBeforeAmbientHome(t *testing.T) {
	repo := makeRepoTree(t)
	t.Setenv("KANDEV_TASK_ID", "")
	ambientHome := filepath.Join(t.TempDir(), "ambient-home")
	explicitDB := filepath.Join(t.TempDir(), "explicit.db")
	cfg := &commonconfig.Config{
		HomeDir:  ambientHome,
		Database: commonconfig.DatabaseConfig{Path: explicitDB},
		Source: commonconfig.ConfigSource{Values: map[string]commonconfig.SettingSource{
			"homeDir":       commonconfig.SourceEnvironment,
			"database.path": commonconfig.SourceEnvironment,
		}},
	}

	dbPath, extra := resolveDevBackendEnv(repo, cfg)
	if dbPath != explicitDB {
		t.Fatalf("dbPath = %q, want explicit environment database %q", dbPath, explicitDB)
	}
	if env := devEnvToMap(extra); env["KANDEV_DATABASE_PATH"] != explicitDB {
		t.Fatalf("KANDEV_DATABASE_PATH = %q, want %q", env["KANDEV_DATABASE_PATH"], explicitDB)
	} else if want := filepath.Join(repo, ".kandev-dev"); env["KANDEV_HOME_DIR"] != want {
		t.Fatalf("KANDEV_HOME_DIR = %q, want repo-local dev home %q", env["KANDEV_HOME_DIR"], want)
	}
}

func TestResolveDevBackendEnvPinsConfiguredHomeForConfiguredDatabase(t *testing.T) {
	repo := makeRepoTree(t)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_HOME_DIR", filepath.Join(t.TempDir(), "ambient-home"))
	configuredHome := filepath.Join(t.TempDir(), "configured-home")
	explicitDB := filepath.Join(t.TempDir(), "configured.db")
	cfg := &commonconfig.Config{
		HomeDir:  configuredHome,
		Database: commonconfig.DatabaseConfig{Path: explicitDB},
		Source: commonconfig.ConfigSource{Values: map[string]commonconfig.SettingSource{
			"homeDir":       commonconfig.SourceConfiguration,
			"database.path": commonconfig.SourceConfiguration,
		}},
	}

	dbPath, extra := resolveDevBackendEnv(repo, cfg)
	if dbPath != explicitDB {
		t.Fatalf("dbPath = %q, want %q", dbPath, explicitDB)
	}
	env := devEnvToMap(extra)
	if env["KANDEV_HOME_DIR"] != configuredHome {
		t.Fatalf("KANDEV_HOME_DIR = %q, want configured dev home %q", env["KANDEV_HOME_DIR"], configuredHome)
	}
}

func TestResolveDevBackendEnvClearsLeakedPathInTaskWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "/leaked/from/parent/kandev.db")

	repo := filepath.Join(home, ".kandev", "tasks", "ws", "repo")
	dbPath, extra := resolveDevBackendEnv(repo)
	env := devEnvToMap(extra)
	wantHome := filepath.Join(repo, ".kandev-dev")

	if dbPath != filepath.Join(wantHome, "data", "kandev.db") {
		t.Fatalf("dbPath = %q, want the repo-local dev db", dbPath)
	}
	if env["KANDEV_HOME_DIR"] != wantHome {
		t.Fatalf("KANDEV_HOME_DIR = %q, want %q", env["KANDEV_HOME_DIR"], wantHome)
	}
	if env["KANDEV_DATABASE_PATH"] != filepath.Join(wantHome, "data", "kandev.db") {
		t.Fatalf("KANDEV_DATABASE_PATH = %q, want repo-local database", env["KANDEV_DATABASE_PATH"])
	}
}

func TestResolveDevBackendEnvTaskWorkspaceIgnoresSelectedDatabase(t *testing.T) {
	root := makeRepoTree(t)
	t.Setenv("KANDEV_TASK_ID", "task-123")
	cfg := &commonconfig.Config{
		Database: commonconfig.DatabaseConfig{Path: filepath.Join(t.TempDir(), "parent.db")},
		Source: commonconfig.ConfigSource{Values: map[string]commonconfig.SettingSource{
			"database.path": commonconfig.SourceConfiguration,
		}},
	}

	dbPath, extra := resolveDevBackendEnv(root, cfg)
	want := filepath.Join(root, ".kandev-dev", "data", "kandev.db")
	if dbPath != want {
		t.Fatalf("dbPath = %q, want task-local %q", dbPath, want)
	}
	if got := devEnvToMap(extra)["KANDEV_DATABASE_PATH"]; got != want {
		t.Fatalf("KANDEV_DATABASE_PATH = %q, want task-local database", got)
	}
}

func TestValidateDevDatabaseTargetRejectsRepoLocalSymlinkEscape(t *testing.T) {
	repo := makeRepoTree(t)
	devHome := devKandevHome(repo)
	if err := os.Symlink(t.TempDir(), devHome); err != nil {
		t.Fatal(err)
	}
	target := devDatabaseTarget{path: filepath.Join(devHome, "data", "kandev.db"), source: devDatabaseDefault}
	if err := validateDevDatabaseTarget(repo, target); err == nil {
		t.Fatal("validateDevDatabaseTarget accepted a symlinked repo-local database path")
	}
}

func TestValidateDevDatabaseTargetRejectsExplicitRepoLocalSymlinkEscape(t *testing.T) {
	repo := makeRepoTree(t)
	devHome := devKandevHome(repo)
	dataDir := filepath.Join(devHome, "data")
	if err := os.MkdirAll(devHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), dataDir); err != nil {
		t.Fatal(err)
	}
	target := devDatabaseTarget{path: filepath.Join(dataDir, "kandev.db"), source: devDatabaseEnvironment}
	if err := validateDevDatabaseTarget(repo, target); err == nil {
		t.Fatal("validateDevDatabaseTarget accepted an explicit database path through a repo-local symlink")
	}
}

func TestValidateDevDatabaseTargetRejectsPathOutsideDataDir(t *testing.T) {
	repo := makeRepoTree(t)
	devHome := devKandevHome(repo)
	dataDir := filepath.Join(devHome, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		path   string
		source devDatabaseSource
	}{
		{
			name:   "repo-local default sibling of data",
			path:   filepath.Join(devHome, "kandev.db"),
			source: devDatabaseDefault,
		},
		{
			name:   "repo-local default under supervisor",
			path:   filepath.Join(devHome, "supervisor", "kandev.db"),
			source: devDatabaseDefault,
		},
		{
			name:   "task source sibling of data",
			path:   filepath.Join(devHome, "kandev.db"),
			source: devDatabaseTask,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := devDatabaseTarget{path: tt.path, source: tt.source}
			if err := validateDevDatabaseTarget(repo, target); err == nil {
				t.Fatal("validateDevDatabaseTarget accepted a database path outside .kandev-dev/data")
			}
		})
	}
}

func TestValidateDevDatabaseTargetRejectsExplicitRepoLocalPathOutsideDataDir(t *testing.T) {
	repo := makeRepoTree(t)
	devHome := devKandevHome(repo)

	for _, source := range []devDatabaseSource{devDatabaseEnvironment, devDatabaseConfiguration} {
		t.Run(string(source), func(t *testing.T) {
			target := devDatabaseTarget{
				path:   filepath.Join(devHome, "supervisor", "kandev.db"),
				source: source,
			}
			if err := validateDevDatabaseTarget(repo, target); err == nil {
				t.Fatal("validateDevDatabaseTarget accepted an explicit repo-local path outside .kandev-dev/data")
			}
		})
	}
}

func TestValidateDevDatabaseTargetAcceptsPathInsideDataDir(t *testing.T) {
	repo := makeRepoTree(t)
	devHome := devKandevHome(repo)
	dataDir := filepath.Join(devHome, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		path   string
		source devDatabaseSource
	}{
		{
			name:   "repo-local default inside data",
			path:   filepath.Join(dataDir, "kandev.db"),
			source: devDatabaseDefault,
		},
		{
			name:   "task source inside data",
			path:   filepath.Join(dataDir, "kandev.db"),
			source: devDatabaseTask,
		},
		{
			name:   "environment source inside data",
			path:   filepath.Join(dataDir, "kandev.db"),
			source: devDatabaseEnvironment,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := devDatabaseTarget{path: tt.path, source: tt.source}
			if err := validateDevDatabaseTarget(repo, target); err != nil {
				t.Fatalf("validateDevDatabaseTarget rejected valid database path: %v", err)
			}
		})
	}
}

func TestNormalizeDevDatabaseTargetMakesPathAbsolute(t *testing.T) {
	relPath := filepath.Join(".kandev-dev", "data", "kandev.db")
	relHome := ".kandev-dev"
	target := devDatabaseTarget{
		path:    relPath,
		homeDir: relHome,
		source:  devDatabaseDefault,
		extra:   []string{"KANDEV_DEBUG_DEV_MODE=true", "KANDEV_HOME_DIR=" + relHome, "KANDEV_DATABASE_PATH=" + relPath},
	}

	normalized, err := normalizeDevDatabaseTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	wantPath, err := filepath.Abs(relPath)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.path != wantPath {
		t.Fatalf("normalized path = %q, want absolute %q", normalized.path, wantPath)
	}
	wantHome, err := filepath.Abs(relHome)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.homeDir != wantHome {
		t.Fatalf("normalized homeDir = %q, want absolute %q", normalized.homeDir, wantHome)
	}

	foundPath := false
	foundHome := false
	for _, item := range normalized.extra {
		switch {
		case strings.HasPrefix(item, "KANDEV_DATABASE_PATH="):
			val := item[len("KANDEV_DATABASE_PATH="):]
			if val != wantPath {
				t.Fatalf("KANDEV_DATABASE_PATH in extra = %q, want %q", val, wantPath)
			}
			foundPath = true
		case strings.HasPrefix(item, "KANDEV_HOME_DIR="):
			val := item[len("KANDEV_HOME_DIR="):]
			if val != wantHome {
				t.Fatalf("KANDEV_HOME_DIR in extra = %q, want %q", val, wantHome)
			}
			foundHome = true
		}
	}
	if !foundPath {
		t.Fatal("KANDEV_DATABASE_PATH not found in normalized extra")
	}
	if !foundHome {
		t.Fatal("KANDEV_HOME_DIR not found in normalized extra")
	}
}

func TestNormalizeDevDatabaseTargetNestedCwdRelativeExplicit(t *testing.T) {
	// Simulate invoking dev from a nested directory like <repo>/apps with a
	// relative explicit database path and relative YAML homeDir. The
	// launcher's CWD is <repo>/apps, but the backend's CWD will be <repo>.
	// filepath.Abs resolves the path relative to CWD; normalization must
	// produce the CWD-absolute form so the child does not resolve it
	// differently.
	repo := makeRepoTree(t)
	nestedCWD := filepath.Join(repo, "apps")
	if err := os.MkdirAll(nestedCWD, 0o700); err != nil {
		t.Fatal(err)
	}
	relativePath := "../.kandev-dev/data/kandev.db"
	relativeHome := "../.kandev-dev"

	// Change to the nested CWD so filepath.Abs resolves relative to it.
	t.Chdir(nestedCWD)

	wantPath, err := filepath.Abs(relativePath)
	if err != nil {
		t.Fatal(err)
	}
	if wantPath != filepath.Join(repo, ".kandev-dev", "data", "kandev.db") {
		t.Fatalf("abs path = %q, want %q", wantPath, filepath.Join(repo, ".kandev-dev", "data", "kandev.db"))
	}
	wantHome, err := filepath.Abs(relativeHome)
	if err != nil {
		t.Fatal(err)
	}
	if wantHome != filepath.Join(repo, ".kandev-dev") {
		t.Fatalf("abs home = %q, want %q", wantHome, filepath.Join(repo, ".kandev-dev"))
	}

	target := devDatabaseTarget{
		path:    relativePath,
		homeDir: relativeHome,
		source:  devDatabaseEnvironment,
		extra:   []string{"KANDEV_DEBUG_DEV_MODE=true", "KANDEV_HOME_DIR=" + relativeHome, "KANDEV_DATABASE_PATH=" + relativePath},
	}

	normalized, err := normalizeDevDatabaseTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.path != wantPath {
		t.Fatalf("normalized path = %q, want %q", normalized.path, wantPath)
	}
	if normalized.homeDir != wantHome {
		t.Fatalf("normalized homeDir = %q, want %q", normalized.homeDir, wantHome)
	}
}

func TestNormalizeDevDatabaseTargetRejectsUnavailableWorkingDirectory(t *testing.T) {
	workingDir := t.TempDir()
	t.Chdir(workingDir)
	absolutePath := filepath.Join(workingDir, "absolute.db")
	if err := os.Remove(workingDir); err != nil {
		t.Skipf("executor does not allow removing the current directory: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "relative database path", path: "relative.db"},
		{name: "absolute database path", path: absolutePath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, err := normalizeDevDatabaseTarget(devDatabaseTarget{
				path:    tt.path,
				homeDir: "relative-home",
				extra:   []string{"KANDEV_DATABASE_PATH=" + tt.path, "KANDEV_HOME_DIR=relative-home"},
			})
			if err == nil {
				t.Fatalf("normalizeDevDatabaseTarget returned %+v after absolute-path resolution failed", normalized)
			}
		})
	}
}
