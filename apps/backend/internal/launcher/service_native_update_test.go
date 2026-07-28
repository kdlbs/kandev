package launcher

//revive:disable:file-length-limit // Native update and privileged install regression coverage is scenario-heavy.

import (
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNativeServiceRenderersIncludeManagedIdentity(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		wants []string
	}{
		{
			name: "systemd",
			text: renderSystemdUnit(nativeServiceUnitInput{
				Executable: "/usr/local/lib/node_modules/@kdlbs/runtime-linux-x64/bin/kandev",
				HomeDir:    "/home/alice/.kandev",
				LogDir:     "/home/alice/.kandev/logs",
			}),
			wants: []string{
				"Environment=KANDEV_RUNNING_AS_SERVICE=true",
				"Environment=KANDEV_SERVICE_MODE=user",
				"Environment=KANDEV_SERVICE_MANAGER=systemd",
				"Environment=KANDEV_INSTALL_KIND=npm",
				"Environment=KANDEV_SERVICE_METADATA=/home/alice/.kandev/service/install.json",
			},
		},
		{
			name: "launchd",
			text: renderLaunchdPlist(nativeServiceUnitInput{
				Executable: "/opt/homebrew/Cellar/kandev/1.2.3/libexec/bin/kandev",
				HomeDir:    "/Users/alice/.kandev",
				LogDir:     "/Users/alice/.kandev/logs",
			}),
			wants: []string{
				"<key>KANDEV_RUNNING_AS_SERVICE</key>\n      <string>true</string>",
				"<key>KANDEV_SERVICE_MODE</key>\n      <string>user</string>",
				"<key>KANDEV_SERVICE_MANAGER</key>\n      <string>launchd</string>",
				"<key>KANDEV_INSTALL_KIND</key>\n      <string>homebrew</string>",
				"<key>KANDEV_SERVICE_METADATA</key>\n      <string>/Users/alice/.kandev/service/install.json</string>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range tt.wants {
				if !strings.Contains(tt.text, want) {
					t.Fatalf("rendered service missing %q:\n%s", want, tt.text)
				}
			}
		})
	}
}

func TestInstallSystemdWritesOwnerOnlyNativeMetadata(t *testing.T) {
	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
	})

	executable := "/opt/homebrew/Cellar/kandev/1.2.3/libexec/bin/kandev"
	executablePath = func() (string, error) { return executable, nil }
	executeServiceCommand = func(string, ...string) error { return nil }
	servicePrintln = func(string) {}
	currentUser, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUDO_USER", currentUser.Username)

	tmp := t.TempDir()
	homeDir := filepath.Join(tmp, "home")
	if err := os.Mkdir(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(tmp, "kandev.service")
	code := installSystemd(
		serviceArgs{
			Action:      actionInstall,
			System:      true,
			HomeDir:     homeDir,
			Port:        38429,
			NoBootStart: true,
		},
		BuildInfo{Version: "1.2.3"},
		unitPath,
	)
	if code != 0 {
		t.Fatalf("installSystemd() = %d, want 0", code)
	}

	metadataPath := filepath.Join(homeDir, "service", "install.json")
	assertNativeInstallMetadata(t, metadataPath, map[string]interface{}{
		"version":          float64(1),
		"manager":          "systemd",
		"mode":             "system",
		"kind":             "homebrew",
		"home_dir":         homeDir,
		"log_dir":          filepath.Join(homeDir, "logs"),
		"service_path":     unitPath,
		"launcher_path":    executable,
		"bundle_dir":       "/opt/homebrew/Cellar/kandev/1.2.3/libexec",
		"launcher_version": "1.2.3",
		"port":             float64(38429),
		"system_user":      currentUser.Username,
		"no_boot_start":    true,
	})
}

func TestInstallLaunchdWritesNativeMetadata(t *testing.T) {
	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
	})

	executable := "/Users/alice/.npm/_npx/abc/node_modules/@kdlbs/runtime-darwin-arm64/bin/kandev"
	executablePath = func() (string, error) { return executable, nil }
	executeServiceCommand = func(name string, args ...string) error {
		if name == "launchctl" && len(args) > 0 && args[0] == "print" {
			return errors.New("not loaded")
		}
		return nil
	}
	servicePrintln = func(string) {}

	tmp := t.TempDir()
	homeDir := filepath.Join(tmp, "home")
	plistPath := filepath.Join(tmp, "com.kdlbs.kandev.plist")
	code := installLaunchd(
		serviceArgs{Action: actionInstall, HomeDir: homeDir, NoBootStart: true},
		BuildInfo{Version: "1.2.3"},
		plistPath,
		"gui/501/com.kdlbs.kandev",
		"gui/501",
	)
	if code != 0 {
		t.Fatalf("installLaunchd() = %d, want 0", code)
	}

	assertNativeInstallMetadata(t, filepath.Join(homeDir, "service", "install.json"), map[string]interface{}{
		"version":          float64(1),
		"manager":          "launchd",
		"mode":             "user",
		"kind":             "npx",
		"home_dir":         homeDir,
		"log_dir":          filepath.Join(homeDir, "logs"),
		"service_path":     plistPath,
		"launcher_path":    executable,
		"bundle_dir":       filepath.Dir(filepath.Dir(executable)),
		"launcher_version": "1.2.3",
		"no_boot_start":    true,
	})
}

func TestInstallLaunchdWaitsForBootoutAndRetriesBootstrap(t *testing.T) {
	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	originalLaunchctlCommand := launchctlCommand
	originalLaunchctlSleep := launchctlSleep
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
		launchctlCommand = originalLaunchctlCommand
		launchctlSleep = originalLaunchctlSleep
	})

	executablePath = func() (string, error) { return "/opt/kandev/bin/kandev", nil }
	executeServiceCommand = func(string, ...string) error { return nil }
	servicePrintln = func(string) {}
	printCalls := 0
	bootstrapCalls := 0
	var order []string
	launchctlSleep = func(time.Duration) {}
	launchctlCommand = func(args ...string) error {
		order = append(order, args[0])
		switch args[0] {
		case "print":
			printCalls++
			if printCalls >= 3 {
				return errors.New("not loaded")
			}
		case "bootstrap":
			bootstrapCalls++
			if bootstrapCalls < 3 {
				return errors.New("Bootstrap failed: 5: Input/output error")
			}
		}
		return nil
	}

	tmp := t.TempDir()
	code := installLaunchd(
		serviceArgs{Action: actionInstall, HomeDir: filepath.Join(tmp, "home")},
		BuildInfo{Version: "test"},
		filepath.Join(tmp, "com.kdlbs.kandev.plist"),
		"gui/501/com.kdlbs.kandev",
		"gui/501",
	)
	if code != 0 {
		t.Fatalf("installLaunchd() = %d, want 0; order=%v", code, order)
	}
	if printCalls != 3 || bootstrapCalls != 3 {
		t.Fatalf("print calls=%d bootstrap calls=%d, want 3 each; order=%v", printCalls, bootstrapCalls, order)
	}
	firstBootstrap := slices.Index(order, "bootstrap")
	lastPrint := firstBootstrap - 1
	if firstBootstrap < 0 || lastPrint < 0 || order[lastPrint] != "print" {
		t.Fatalf("bootstrap started before teardown polling completed: %v", order)
	}
}

func TestReloadLaunchdServiceStopsWhenBootoutTimesOut(t *testing.T) {
	originalLaunchctlCommand := launchctlCommand
	originalLaunchctlSleep := launchctlSleep
	t.Cleanup(func() {
		launchctlCommand = originalLaunchctlCommand
		launchctlSleep = originalLaunchctlSleep
	})

	bootstrapCalls := 0
	launchctlSleep = func(time.Duration) {}
	launchctlCommand = func(args ...string) error {
		if args[0] == "bootstrap" {
			bootstrapCalls++
		}
		return nil
	}

	err := reloadLaunchdService(
		"gui/501/com.kdlbs.kandev",
		"gui/501",
		"/Users/alice/Library/LaunchAgents/com.kdlbs.kandev.plist",
	)
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for launchctl bootout") {
		t.Fatalf("reloadLaunchdService() error = %v, want bootout timeout", err)
	}
	if bootstrapCalls != 0 {
		t.Fatalf("bootstrap called %d times after bootout timeout, want 0", bootstrapCalls)
	}
}

func TestNativeServiceSelfUpdateRunsSupportedPlans(t *testing.T) {
	originalExecuteServiceCommand := executeServiceCommand
	t.Cleanup(func() { executeServiceCommand = originalExecuteServiceCommand })

	tests := []struct {
		name     string
		install  map[string]interface{}
		expected []string
	}{
		{
			name: "homebrew",
			install: map[string]interface{}{
				"manager": "launchd", "mode": "user", "kind": "homebrew",
				"home_dir": "/home/alice/.kandev", "launcher_path": "/opt/homebrew/Cellar/kandev/1.2.2/libexec/bin/kandev",
				"port": 38429, "no_boot_start": true,
			},
			expected: []string{
				"brew upgrade kandev",
				"kandev service install --home-dir /home/alice/.kandev --port 38429 --no-boot-start",
				"launchctl kickstart -k gui/" + strconv.Itoa(os.Getuid()) + "/com.kdlbs.kandev",
			},
		},
		{
			name: "npm",
			install: map[string]interface{}{
				"manager": "systemd", "mode": "user", "kind": "npm",
				"home_dir":      "/home/alice/.kandev",
				"launcher_path": "/usr/local/lib/node_modules/@kdlbs/runtime-linux-x64/bin/kandev",
				"no_boot_start": true,
			},
			expected: []string{
				"npm install -g --prefix /usr/local kandev@1.2.3",
				"/usr/local/lib/node_modules/@kdlbs/runtime-linux-x64/bin/kandev service install --home-dir /home/alice/.kandev --no-boot-start",
				"systemctl --user restart kandev.service",
			},
		},
		{
			name: "npx",
			install: map[string]interface{}{
				"manager": "systemd", "mode": "user", "kind": "npx",
				"home_dir":      "/home/alice/.kandev",
				"launcher_path": "/home/alice/.npm/_npx/abc/node_modules/@kdlbs/runtime-linux-x64/bin/kandev",
			},
			expected: []string{
				"npx -y kandev@1.2.3 service install --home-dir /home/alice/.kandev",
				"systemctl --user restart kandev.service",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intentPath := writeNativeUpdateIntent(t, tt.install)
			var commands []string
			executeServiceCommand = func(name string, args ...string) error {
				commands = append(commands, name+" "+strings.Join(args, " "))
				return nil
			}

			if code := runService([]string{"self-update", "--intent", intentPath}, BuildInfo{}); code != 0 {
				t.Fatalf("runService(self-update) = %d, want 0", code)
			}
			if !slices.Equal(commands, tt.expected) {
				t.Fatalf("commands = %#v, want %#v", commands, tt.expected)
			}
		})
	}
}

func TestNativeServiceSelfUpdateClearsStaleLauncherEnvironment(t *testing.T) {
	originalExecuteServiceCommand := executeServiceCommand
	t.Cleanup(func() { executeServiceCommand = originalExecuteServiceCommand })
	t.Setenv("KANDEV_VERSION", "0.81.0")
	t.Setenv("KANDEV_BUNDLE_DIR", "/old/kandev/libexec")

	tests := []struct {
		name             string
		install          map[string]interface{}
		reinstallCommand string
	}{
		{
			name: "homebrew wrapper can provide fresh values",
			install: map[string]interface{}{
				"manager": "systemd", "mode": "user", "kind": "homebrew",
				"home_dir": "/home/alice/.kandev", "launcher_path": "/opt/homebrew/Cellar/kandev/0.81.0/libexec/bin/kandev",
			},
			reinstallCommand: "kandev",
		},
		{
			name: "npm native launcher",
			install: map[string]interface{}{
				"manager": "systemd", "mode": "user", "kind": "npm",
				"home_dir":      "/home/alice/.kandev",
				"launcher_path": "/usr/local/lib/node_modules/@kdlbs/runtime-linux-x64/bin/kandev",
			},
			reinstallCommand: "/usr/local/lib/node_modules/@kdlbs/runtime-linux-x64/bin/kandev",
		},
		{
			name: "npx package launcher",
			install: map[string]interface{}{
				"manager": "systemd", "mode": "user", "kind": "npx",
				"home_dir":      "/home/alice/.kandev",
				"launcher_path": "/home/alice/.npm/_npx/abc/node_modules/@kdlbs/runtime-linux-x64/bin/kandev",
			},
			reinstallCommand: "npx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intentPath := writeNativeUpdateIntent(t, tt.install)
			var staleVersion, staleBundle string
			executeServiceCommand = func(name string, _ ...string) error {
				if name == tt.reinstallCommand {
					staleVersion = os.Getenv("KANDEV_VERSION")
					staleBundle = os.Getenv("KANDEV_BUNDLE_DIR")
				}
				return nil
			}

			if code := runService([]string{"self-update", "--intent", intentPath}, BuildInfo{}); code != 0 {
				t.Fatalf("runService(self-update) = %d, want 0", code)
			}
			if staleVersion != "" || staleBundle != "" {
				t.Fatalf("reinstall inherited stale launcher env: version=%q bundle=%q", staleVersion, staleBundle)
			}
			if got := os.Getenv("KANDEV_VERSION"); got != "0.81.0" {
				t.Fatalf("KANDEV_VERSION was not restored, got %q", got)
			}
			if got := os.Getenv("KANDEV_BUNDLE_DIR"); got != "/old/kandev/libexec" {
				t.Fatalf("KANDEV_BUNDLE_DIR was not restored, got %q", got)
			}
		})
	}
}

func TestInstallSystemdRejectsSymlinkedMetadataPathsBeforeReload(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, homeDir, outside string)
	}{
		{
			name: "service directory",
			setup: func(t *testing.T, homeDir, outside string) {
				t.Helper()
				if err := os.Symlink(outside, filepath.Join(homeDir, "service")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "install json",
			setup: func(t *testing.T, homeDir, outside string) {
				t.Helper()
				serviceDir := filepath.Join(homeDir, "service")
				if err := os.Mkdir(serviceDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "escaped.json"), filepath.Join(serviceDir, "install.json")); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalExecutablePath := executablePath
			originalExecuteServiceCommand := executeServiceCommand
			originalServicePrintln := servicePrintln
			t.Cleanup(func() {
				executablePath = originalExecutablePath
				executeServiceCommand = originalExecuteServiceCommand
				servicePrintln = originalServicePrintln
			})

			tmp := t.TempDir()
			homeDir := filepath.Join(tmp, "home")
			outside := filepath.Join(tmp, "outside")
			if err := os.MkdirAll(homeDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(outside, 0o700); err != nil {
				t.Fatal(err)
			}
			tt.setup(t, homeDir, outside)
			t.Setenv("SUDO_USER", currentUser.Username)

			executablePath = func() (string, error) { return "/opt/kandev/bin/kandev", nil }
			servicePrintln = func(string) {}
			var managerCommands []string
			executeServiceCommand = func(name string, args ...string) error {
				managerCommands = append(managerCommands, name+" "+strings.Join(args, " "))
				return nil
			}

			code := installSystemd(
				serviceArgs{Action: actionInstall, System: true, HomeDir: homeDir},
				BuildInfo{Version: "test"},
				filepath.Join(tmp, "kandev.service"),
			)
			if code == 0 {
				t.Fatalf("installSystemd() = 0 for symlinked metadata path")
			}
			if len(managerCommands) != 0 {
				t.Fatalf("manager commands ran after metadata failure: %v", managerCommands)
			}
			if _, err := os.Stat(filepath.Join(outside, "install.json")); !os.IsNotExist(err) {
				t.Fatalf("service symlink escape was written, stat err=%v", err)
			}
			if _, err := os.Stat(filepath.Join(outside, "escaped.json")); !os.IsNotExist(err) {
				t.Fatalf("install.json symlink escape was written, stat err=%v", err)
			}
		})
	}
}

func TestInstallSystemdRejectsSymlinkedHomeBeforeMetadataMutation(t *testing.T) {
	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	originalLookup := lookupNativeServiceOwner
	originalChown := chownNativeServiceMetadata
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
		lookupNativeServiceOwner = originalLookup
		chownNativeServiceMetadata = originalChown
	})

	tmp := t.TempDir()
	outside := filepath.Join(tmp, "outside")
	outsideService := filepath.Join(outside, "service")
	if err := os.MkdirAll(outsideService, 0o755); err != nil {
		t.Fatal(err)
	}
	homeDir := filepath.Join(tmp, "linked-home")
	if err := os.Symlink(outside, homeDir); err != nil {
		t.Fatal(err)
	}

	lookupNativeServiceOwner = func(string) (int, int, error) { return 1234, 5678, nil }
	chownCalls := 0
	chownNativeServiceMetadata = func(*os.File, int, int) error {
		chownCalls++
		return nil
	}
	executablePath = func() (string, error) { return "/opt/kandev/bin/kandev", nil }
	servicePrintln = func(string) {}
	var managerCommands []string
	executeServiceCommand = func(name string, args ...string) error {
		managerCommands = append(managerCommands, name+" "+strings.Join(args, " "))
		return nil
	}
	t.Setenv("SUDO_USER", "alice")

	code := installSystemd(
		serviceArgs{Action: actionInstall, System: true, HomeDir: homeDir},
		BuildInfo{Version: "test"},
		filepath.Join(tmp, "kandev.service"),
	)
	if code == 0 {
		t.Fatalf("installSystemd() = 0 for symlinked home dir")
	}
	if len(managerCommands) != 0 {
		t.Fatalf("manager commands ran after unsafe home rejection: %v", managerCommands)
	}
	if chownCalls != 0 {
		t.Fatalf("ownership changed through unsafe home: chown calls=%d", chownCalls)
	}
	if mode := mustStat(t, outsideService).Mode().Perm(); mode != 0o755 {
		t.Fatalf("outside service dir mode = %o, want unchanged 755", mode)
	}
	if _, err := os.Stat(filepath.Join(outsideService, "install.json")); !os.IsNotExist(err) {
		t.Fatalf("metadata escaped through home symlink, stat err=%v", err)
	}
}

func TestInstallSystemdRejectsMissingSystemHomeWithoutCreatingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("system service metadata is unsupported on Windows")
	}
	t.Run("home walker returns actionable error", func(t *testing.T) {
		missingParent := filepath.Join(t.TempDir(), "missing-parent")
		homeDir := filepath.Join(missingParent, "kandev-home")
		root, err := openSystemNativeMetadataHome(homeDir)
		if root != nil {
			_ = root.Close()
		}
		if err == nil {
			t.Fatalf("openSystemNativeMetadataHome() succeeded for missing home")
		}
		if !strings.Contains(err.Error(), "pre-create") || !strings.Contains(err.Error(), "chown") {
			t.Fatalf("error = %q, want actionable pre-create/chown guidance", err)
		}
		if _, statErr := os.Stat(missingParent); !os.IsNotExist(statErr) {
			t.Fatalf("missing system home path was partially created, stat err=%v", statErr)
		}
	})

	t.Run("install stops before manager reload", func(t *testing.T) {
		originalExecutablePath := executablePath
		originalExecuteServiceCommand := executeServiceCommand
		originalServicePrintln := servicePrintln
		originalLookup := lookupNativeServiceOwner
		originalChown := chownNativeServiceMetadata
		t.Cleanup(func() {
			executablePath = originalExecutablePath
			executeServiceCommand = originalExecuteServiceCommand
			servicePrintln = originalServicePrintln
			lookupNativeServiceOwner = originalLookup
			chownNativeServiceMetadata = originalChown
		})

		missingParent := filepath.Join(t.TempDir(), "missing-parent")
		homeDir := filepath.Join(missingParent, "kandev-home")
		executablePath = func() (string, error) { return "/opt/kandev/bin/kandev", nil }
		servicePrintln = func(string) {}
		lookupNativeServiceOwner = func(string) (int, int, error) { return 1234, 5678, nil }
		chownNativeServiceMetadata = func(*os.File, int, int) error { return nil }
		var managerCommands []string
		executeServiceCommand = func(name string, args ...string) error {
			managerCommands = append(managerCommands, name+" "+strings.Join(args, " "))
			return nil
		}
		t.Setenv("SUDO_USER", "alice")

		code := installSystemd(
			serviceArgs{Action: actionInstall, System: true, HomeDir: homeDir},
			BuildInfo{Version: "test"},
			filepath.Join(t.TempDir(), "kandev.service"),
		)
		if code == 0 {
			t.Fatalf("installSystemd() = 0 for missing system home")
		}
		if len(managerCommands) != 0 {
			t.Fatalf("manager commands ran after missing home rejection: %v", managerCommands)
		}
		if _, err := os.Stat(missingParent); !os.IsNotExist(err) {
			t.Fatalf("missing system home path was partially created, stat err=%v", err)
		}
	})
}

func TestInstallSystemdPreflightPreservesDefinitionForUnsafeSystemHome(t *testing.T) {
	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	originalLookup := lookupNativeServiceOwner
	originalChown := chownNativeServiceMetadata
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
		lookupNativeServiceOwner = originalLookup
		chownNativeServiceMetadata = originalChown
	})

	tests := []struct {
		name  string
		setup func(t *testing.T, tmp string) (homeDir, untouchedRoot string)
	}{
		{
			name: "missing home",
			setup: func(t *testing.T, tmp string) (string, string) {
				t.Helper()
				missingRoot := filepath.Join(tmp, "missing-parent")
				return filepath.Join(missingRoot, "home"), missingRoot
			},
		},
		{
			name: "symlinked home",
			setup: func(t *testing.T, tmp string) (string, string) {
				t.Helper()
				outside := filepath.Join(tmp, "outside")
				if err := os.Mkdir(outside, 0o700); err != nil {
					t.Fatal(err)
				}
				homeDir := filepath.Join(tmp, "linked-home")
				if err := os.Symlink(outside, homeDir); err != nil {
					t.Fatal(err)
				}
				return homeDir, outside
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			homeDir, untouchedRoot := tt.setup(t, tmp)
			unitPath := filepath.Join(tmp, "kandev.service")
			originalDefinition := "# managed by kandev\noriginal systemd definition\n"
			if err := os.WriteFile(unitPath, []byte(originalDefinition), 0o640); err != nil {
				t.Fatal(err)
			}

			executablePath = func() (string, error) { return "/opt/kandev/bin/kandev", nil }
			servicePrintln = func(string) {}
			lookupNativeServiceOwner = func(string) (int, int, error) { return 1234, 5678, nil }
			chownNativeServiceMetadata = func(*os.File, int, int) error { return nil }
			var managerCommands []string
			executeServiceCommand = func(name string, args ...string) error {
				managerCommands = append(managerCommands, name+" "+strings.Join(args, " "))
				return nil
			}
			t.Setenv("SUDO_USER", "alice")

			code := installSystemd(
				serviceArgs{Action: actionInstall, System: true, HomeDir: homeDir},
				BuildInfo{Version: "test"},
				unitPath,
			)
			if code == 0 {
				t.Fatalf("installSystemd() = 0 for unsafe system home")
			}
			assertDefinitionUnchanged(t, unitPath, originalDefinition, 0o640)
			if len(managerCommands) != 0 {
				t.Fatalf("manager commands ran after preflight failure: %v", managerCommands)
			}
			if _, err := os.Stat(filepath.Join(untouchedRoot, "service", "install.json")); !os.IsNotExist(err) {
				t.Fatalf("metadata was created through unsafe home, stat err=%v", err)
			}
		})
	}
}

func TestInstallLaunchdPreflightPreservesDefinitionAndLogsForUnsafeSystemHome(t *testing.T) {
	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	originalLookup := lookupNativeServiceOwner
	originalChown := chownNativeServiceMetadata
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
		lookupNativeServiceOwner = originalLookup
		chownNativeServiceMetadata = originalChown
	})

	tests := []struct {
		name  string
		setup func(t *testing.T, tmp string) (homeDir, untouchedRoot string)
	}{
		{
			name: "missing home",
			setup: func(t *testing.T, tmp string) (string, string) {
				t.Helper()
				missingRoot := filepath.Join(tmp, "missing-parent")
				return filepath.Join(missingRoot, "home"), missingRoot
			},
		},
		{
			name: "symlinked home",
			setup: func(t *testing.T, tmp string) (string, string) {
				t.Helper()
				outside := filepath.Join(tmp, "outside")
				if err := os.Mkdir(outside, 0o700); err != nil {
					t.Fatal(err)
				}
				homeDir := filepath.Join(tmp, "linked-home")
				if err := os.Symlink(outside, homeDir); err != nil {
					t.Fatal(err)
				}
				return homeDir, outside
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			homeDir, untouchedRoot := tt.setup(t, tmp)
			plistPath := filepath.Join(tmp, "com.kdlbs.kandev.plist")
			originalDefinition := "<!-- managed by kandev -->\noriginal launchd definition\n"
			if err := os.WriteFile(plistPath, []byte(originalDefinition), 0o640); err != nil {
				t.Fatal(err)
			}

			executablePath = func() (string, error) { return "/opt/kandev/bin/kandev", nil }
			servicePrintln = func(string) {}
			lookupNativeServiceOwner = func(string) (int, int, error) { return 1234, 5678, nil }
			chownNativeServiceMetadata = func(*os.File, int, int) error { return nil }
			var managerCommands []string
			executeServiceCommand = func(name string, args ...string) error {
				managerCommands = append(managerCommands, name+" "+strings.Join(args, " "))
				return nil
			}
			t.Setenv("SUDO_USER", "alice")

			code := installLaunchd(
				serviceArgs{Action: actionInstall, System: true, HomeDir: homeDir},
				BuildInfo{Version: "test"},
				plistPath,
				"system/com.kdlbs.kandev",
				"system",
			)
			if code == 0 {
				t.Fatalf("installLaunchd() = 0 for unsafe system home")
			}
			assertDefinitionUnchanged(t, plistPath, originalDefinition, 0o640)
			if len(managerCommands) != 0 {
				t.Fatalf("manager commands ran after preflight failure: %v", managerCommands)
			}
			for _, escaped := range []string{
				filepath.Join(untouchedRoot, "logs"),
				filepath.Join(untouchedRoot, "service", "install.json"),
			} {
				if _, err := os.Stat(escaped); !os.IsNotExist(err) {
					t.Fatalf("path was created through unsafe home %q, stat err=%v", escaped, err)
				}
			}
		})
	}
}

func TestInstallLaunchdRejectsSymlinkedSystemLogsBeforeDefinitionWrite(t *testing.T) {
	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	originalLookup := lookupNativeServiceOwner
	originalChown := chownNativeServiceMetadata
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
		lookupNativeServiceOwner = originalLookup
		chownNativeServiceMetadata = originalChown
	})

	tmp := t.TempDir()
	homeDir := filepath.Join(tmp, "home")
	outside := filepath.Join(tmp, "outside")
	if err := os.Mkdir(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(homeDir, "logs")); err != nil {
		t.Fatal(err)
	}
	plistPath := filepath.Join(tmp, "com.kdlbs.kandev.plist")
	originalDefinition := "<!-- managed by kandev -->\noriginal launchd definition\n"
	if err := os.WriteFile(plistPath, []byte(originalDefinition), 0o640); err != nil {
		t.Fatal(err)
	}

	executablePath = func() (string, error) { return "/opt/kandev/bin/kandev", nil }
	servicePrintln = func(string) {}
	lookupNativeServiceOwner = func(string) (int, int, error) { return 1234, 5678, nil }
	chownCalls := 0
	chownNativeServiceMetadata = func(*os.File, int, int) error {
		chownCalls++
		return nil
	}
	var managerCommands []string
	executeServiceCommand = func(name string, args ...string) error {
		managerCommands = append(managerCommands, name+" "+strings.Join(args, " "))
		return nil
	}
	t.Setenv("SUDO_USER", "alice")

	code := installLaunchd(
		serviceArgs{Action: actionInstall, System: true, HomeDir: homeDir},
		BuildInfo{Version: "test"},
		plistPath,
		"system/com.kdlbs.kandev",
		"system",
	)
	if code == 0 {
		t.Fatalf("installLaunchd() = 0 for symlinked system logs")
	}
	assertDefinitionUnchanged(t, plistPath, originalDefinition, 0o640)
	if len(managerCommands) != 0 {
		t.Fatalf("manager commands ran after log preflight failure: %v", managerCommands)
	}
	if chownCalls != 0 {
		t.Fatalf("ownership changed through symlinked logs: chown calls=%d", chownCalls)
	}
	if mode := mustStat(t, outside).Mode().Perm(); mode != 0o700 {
		t.Fatalf("outside directory mode = %o, want unchanged 700", mode)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "service")); !os.IsNotExist(err) {
		t.Fatalf("metadata path was partially created after log preflight failure, stat err=%v", err)
	}
}

func TestSystemMetadataOwnershipUsesResolvedServiceUser(t *testing.T) {
	originalLookup := lookupNativeServiceOwner
	originalChown := chownNativeServiceMetadata
	t.Cleanup(func() {
		lookupNativeServiceOwner = originalLookup
		chownNativeServiceMetadata = originalChown
	})

	lookupNativeServiceOwner = func(username string) (int, int, error) {
		if username != "alice" {
			t.Fatalf("lookup username = %q, want alice", username)
		}
		return 1234, 5678, nil
	}
	var ownershipTargets []string
	chownNativeServiceMetadata = func(file *os.File, uid, gid int) error {
		if uid != 1234 || gid != 5678 {
			t.Fatalf("ownership = %d:%d, want 1234:5678", uid, gid)
		}
		ownershipTargets = append(ownershipTargets, file.Name())
		return nil
	}

	homeDir := t.TempDir()
	err := writeNativeServiceMetadata(nativeServiceMetadata{
		Version:      nativeServiceMetadataVersion,
		Mode:         nativeServiceModeSystem,
		SystemUser:   "alice",
		HomeDir:      homeDir,
		ServicePath:  filepath.Join(homeDir, "kandev.service"),
		LauncherPath: "/opt/kandev/bin/kandev",
		InstalledAt:  "2026-07-25T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ownershipTargets) != 2 {
		t.Fatalf("ownership targets = %v, want service dir and install.json", ownershipTargets)
	}
	assertMetadataPermissions(t, filepath.Join(homeDir, "service", "install.json"))
}

func TestSystemMetadataAtomicFailurePreservesExistingFile(t *testing.T) {
	originalLookup := lookupNativeServiceOwner
	originalChown := chownNativeServiceMetadata
	t.Cleanup(func() {
		lookupNativeServiceOwner = originalLookup
		chownNativeServiceMetadata = originalChown
	})

	lookupNativeServiceOwner = func(string) (int, int, error) { return 1234, 5678, nil }
	chownCalls := 0
	chownNativeServiceMetadata = func(*os.File, int, int) error {
		chownCalls++
		if chownCalls == 2 {
			return errors.New("injected file ownership failure")
		}
		return nil
	}

	homeDir := t.TempDir()
	serviceDir := filepath.Join(homeDir, "service")
	if err := os.Mkdir(serviceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(serviceDir, "install.json")
	if err := os.WriteFile(metadataPath, []byte("existing metadata\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := writeNativeServiceMetadata(nativeServiceMetadata{
		Version:      nativeServiceMetadataVersion,
		Mode:         nativeServiceModeSystem,
		SystemUser:   "alice",
		HomeDir:      homeDir,
		ServicePath:  filepath.Join(homeDir, "kandev.service"),
		LauncherPath: "/opt/kandev/bin/kandev",
		InstalledAt:  "2026-07-25T00:00:00Z",
	})
	if err == nil {
		t.Fatalf("writeNativeServiceMetadata() succeeded after ownership failure")
	}
	data, readErr := os.ReadFile(metadataPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "existing metadata\n" {
		t.Fatalf("metadata = %q, want existing contents", data)
	}
	entries, readErr := os.ReadDir(serviceDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 1 || entries[0].Name() != "install.json" {
		t.Fatalf("temporary metadata was not cleaned up: %v", entries)
	}
}

func TestNativeSelfUpdateRemainsHiddenFromServiceHelp(t *testing.T) {
	if strings.Contains(serviceHelp, actionSelfUpdate) {
		t.Fatalf("public service help exposes hidden action:\n%s", serviceHelp)
	}
}

func TestNativeServiceSelfUpdateRejectsUnsupportedKind(t *testing.T) {
	intentPath := writeNativeUpdateIntent(t, map[string]interface{}{
		"manager": "systemd", "mode": "user", "kind": "local",
		"home_dir": t.TempDir(), "launcher_path": "/tmp/kandev",
	})
	if code := runService([]string{"self-update", "--intent", intentPath}, BuildInfo{}); code == 0 {
		t.Fatalf("runService(self-update) = 0 for unsupported install kind")
	}
}

func assertNativeInstallMetadata(t *testing.T, path string, expected map[string]interface{}) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	for key, want := range expected {
		if got[key] != want {
			t.Fatalf("metadata[%q] = %#v, want %#v; metadata=%v", key, got[key], want, got)
		}
	}
	if installedAt, _ := got["installed_at"].(string); installedAt == "" {
		t.Fatalf("metadata missing installed_at: %v", got)
	} else if _, err := time.Parse(time.RFC3339Nano, installedAt); err != nil {
		t.Fatalf("installed_at = %q: %v", installedAt, err)
	}
	assertMetadataPermissions(t, path)
}

func assertMetadataPermissions(t *testing.T, path string) {
	t.Helper()
	if mode := mustStat(t, path).Mode().Perm(); mode != 0o600 {
		t.Fatalf("metadata mode = %o, want 600", mode)
	}
	if mode := mustStat(t, filepath.Dir(path)).Mode().Perm(); mode != 0o700 {
		t.Fatalf("metadata dir mode = %o, want 700", mode)
	}
}

func assertDefinitionUnchanged(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("definition changed:\n%s", data)
	}
	if got := mustStat(t, path).Mode().Perm(); got != mode {
		t.Fatalf("definition mode = %o, want %o", got, mode)
	}
}

func writeNativeUpdateIntent(t *testing.T, install map[string]interface{}) string {
	t.Helper()
	intent := map[string]interface{}{
		"version":        1,
		"target_tag":     "v1.2.3",
		"target_version": "1.2.3",
		"install":        install,
		"created_at":     "2026-07-25T00:00:00Z",
	}
	data, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "intent.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
