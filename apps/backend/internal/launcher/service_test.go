package launcher

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestRenderSystemdUnitExecsNativeKandev(t *testing.T) {
	unit := renderSystemdUnit(nativeServiceUnitInput{
		Executable: "/opt/kandev/bin/kandev",
		HomeDir:    "/home/alice/.kandev",
		LogDir:     "/home/alice/.kandev/logs",
		Port:       1234,
	})
	if !strings.Contains(unit, "ExecStart=/opt/kandev/bin/kandev --headless") {
		t.Fatalf("unit does not exec native kandev:\n%s", unit)
	}
	if strings.Contains(unit, "cli.js") || strings.Contains(unit, "node ") {
		t.Fatalf("unit contains Node CLI launcher:\n%s", unit)
	}
	if !strings.Contains(unit, "Environment=KANDEV_SERVER_PORT=1234") {
		t.Fatalf("unit missing port env:\n%s", unit)
	}
}

func TestRenderSystemdUnitIncludesBundleMetadata(t *testing.T) {
	unit := renderSystemdUnit(nativeServiceUnitInput{
		Executable: "/opt/kandev/bin/kandev",
		HomeDir:    "/home/alice/.kandev",
		LogDir:     "/home/alice/.kandev/logs",
		BundleDir:  "/opt/kandev",
		Version:    "1.2.3",
	})

	if !strings.Contains(unit, "Environment=KANDEV_BUNDLE_DIR=/opt/kandev") {
		t.Fatalf("unit missing bundle dir env:\n%s", unit)
	}
	if !strings.Contains(unit, "Environment=KANDEV_VERSION=1.2.3") {
		t.Fatalf("unit missing version env:\n%s", unit)
	}
}

func TestRenderLaunchdPlistExecsNativeKandev(t *testing.T) {
	plist := renderLaunchdPlist(nativeServiceUnitInput{
		Executable: "/opt/kandev/bin/kandev",
		HomeDir:    "/Users/alice/.kandev",
		LogDir:     "/Users/alice/.kandev/logs",
	})
	if !strings.Contains(plist, "<string>/opt/kandev/bin/kandev</string>") {
		t.Fatalf("plist does not exec native kandev:\n%s", plist)
	}
	if strings.Contains(plist, "cli.js") {
		t.Fatalf("plist contains Node CLI launcher:\n%s", plist)
	}
	if !strings.Contains(plist, "<key>RunAtLoad</key>\n  <true/>") {
		t.Fatalf("plist should start at load by default:\n%s", plist)
	}
}

func TestRenderLaunchdPlistIncludesBundleMetadata(t *testing.T) {
	plist := renderLaunchdPlist(nativeServiceUnitInput{
		Executable: "/opt/kandev/bin/kandev",
		HomeDir:    "/Users/alice/.kandev",
		LogDir:     "/Users/alice/.kandev/logs",
		BundleDir:  "/opt/kandev",
		Version:    "1.2.3",
	})

	if !strings.Contains(plist, "<key>KANDEV_BUNDLE_DIR</key>\n      <string>/opt/kandev</string>") {
		t.Fatalf("plist missing bundle dir env:\n%s", plist)
	}
	if !strings.Contains(plist, "<key>KANDEV_VERSION</key>\n      <string>1.2.3</string>") {
		t.Fatalf("plist missing version env:\n%s", plist)
	}
}

func TestRenderLaunchdPlistCanDisableBootStart(t *testing.T) {
	plist := renderLaunchdPlist(nativeServiceUnitInput{
		Executable:  "/opt/kandev/bin/kandev",
		HomeDir:     "/Users/alice/.kandev",
		LogDir:      "/Users/alice/.kandev/logs",
		NoBootStart: true,
	})

	if !strings.Contains(plist, "<key>RunAtLoad</key>\n  <false/>") {
		t.Fatalf("plist should not start at load with no boot start:\n%s", plist)
	}
}

func TestInstallSystemdMessagesDistinguishBootStart(t *testing.T) {
	tests := []struct {
		name            string
		noBootStart     bool
		wantMessage     string
		wantEnableNow   bool
		wantStartAction bool
	}{
		{
			name:          "default install",
			wantMessage:   "[kandev] service enabled and started",
			wantEnableNow: true,
		},
		{
			name:            "no boot start",
			noBootStart:     true,
			wantMessage:     "[kandev] service installed and started (boot-start disabled)",
			wantStartAction: true,
		},
	}

	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
	})

	executablePath = func() (string, error) {
		return "/opt/kandev/bin/kandev", nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var commands []string
			var messages []string
			executeServiceCommand = func(name string, args ...string) error {
				commands = append(commands, name+" "+strings.Join(args, " "))
				return nil
			}
			servicePrintln = func(message string) {
				messages = append(messages, message)
			}

			tmp := t.TempDir()
			code := installSystemd(
				serviceArgs{
					Action:      actionInstall,
					HomeDir:     filepath.Join(tmp, "home"),
					NoBootStart: tt.noBootStart,
				},
				BuildInfo{Version: "test"},
				filepath.Join(tmp, "kandev.service"),
			)
			if code != 0 {
				t.Fatalf("installSystemd() = %d, want 0", code)
			}
			if !slices.Equal(messages, []string{tt.wantMessage}) {
				t.Fatalf("messages = %v, want %q", messages, tt.wantMessage)
			}
			gotEnableNow := slices.Contains(commands, "systemctl --user enable --now kandev.service")
			if gotEnableNow != tt.wantEnableNow {
				t.Fatalf("enable --now command presence = %v, want %v; commands=%v", gotEnableNow, tt.wantEnableNow, commands)
			}
			gotStartAction := slices.Contains(commands, "systemctl --user start kandev.service")
			if gotStartAction != tt.wantStartAction {
				t.Fatalf("start command presence = %v, want %v; commands=%v", gotStartAction, tt.wantStartAction, commands)
			}
		})
	}
}

func TestInstallSystemdIncludesActiveNodeBinInPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("systemd service PATH rendering is POSIX-only")
	}
	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
	})

	tmp := t.TempDir()
	nodeBin := filepath.Join(tmp, "home", "alice", ".nvm", "versions", "node", "v25.2.1", "bin")
	if err := os.MkdirAll(nodeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"node", "npm", "npx"} {
		if err := os.WriteFile(filepath.Join(nodeBin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", nodeBin+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("npm_node_execpath", filepath.Join(nodeBin, "node"))

	executablePath = func() (string, error) {
		return filepath.Join(tmp, ".npm", "_npx", "abc", "node_modules", "@kdlbs", "runtime-linux-x64", "bin", "kandev"), nil
	}
	executeServiceCommand = func(string, ...string) error { return nil }
	servicePrintln = func(string) {}

	unitPath := filepath.Join(tmp, "kandev.service")
	code := installSystemd(
		serviceArgs{
			Action:      actionInstall,
			HomeDir:     filepath.Join(tmp, "kandev-home"),
			NoBootStart: true,
		},
		BuildInfo{Version: "test"},
		unitPath,
	)
	if code != 0 {
		t.Fatalf("installSystemd() = %d, want 0", code)
	}

	unit, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := "Environment=PATH=" + nodeBin + ":"
	if !strings.Contains(string(unit), wantPrefix) {
		t.Fatalf("unit PATH does not include active node bin %q:\n%s", nodeBin, unit)
	}
}

func TestInstallLaunchdIncludesActiveNodeBinInPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("launchd service PATH rendering is POSIX-only")
	}
	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
	})

	tmp := t.TempDir()
	nodeBin := filepath.Join(tmp, "Users", "alice", ".nvm", "versions", "node", "v25.2.1", "bin")
	if err := os.MkdirAll(nodeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"node", "npm", "npx"} {
		if err := os.WriteFile(filepath.Join(nodeBin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", nodeBin+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("npm_node_execpath", filepath.Join(nodeBin, "node"))

	executablePath = func() (string, error) {
		return filepath.Join(tmp, ".npm", "_npx", "abc", "node_modules", "@kdlbs", "runtime-darwin-arm64", "bin", "kandev"), nil
	}
	executeServiceCommand = func(string, ...string) error { return nil }
	servicePrintln = func(string) {}

	plistPath := filepath.Join(tmp, "com.kdlbs.kandev.plist")
	code := installLaunchd(
		serviceArgs{Action: actionInstall, HomeDir: filepath.Join(tmp, "kandev-home")},
		BuildInfo{Version: "test"},
		plistPath,
		"gui/501/com.kdlbs.kandev",
		"gui/501",
	)
	if code != 0 {
		t.Fatalf("installLaunchd() = %d, want 0", code)
	}

	plist, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := "<key>PATH</key>\n      <string>" + nodeBin + ":"
	if !strings.Contains(string(plist), wantPath) {
		t.Fatalf("plist PATH does not include active node bin %q:\n%s", nodeBin, plist)
	}
}

func TestInstallLaunchdMessagesDistinguishBootStart(t *testing.T) {
	tests := []struct {
		name          string
		noBootStart   bool
		wantMessage   string
		wantKickstart bool
	}{
		{
			name:          "default install",
			wantMessage:   "[kandev] service loaded, enabled, and started",
			wantKickstart: false,
		},
		{
			name:          "no boot start",
			noBootStart:   true,
			wantMessage:   "[kandev] service loaded and started (boot-start disabled)",
			wantKickstart: true,
		},
	}

	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
	})

	executablePath = func() (string, error) {
		return "/opt/kandev/bin/kandev", nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var commands []string
			var messages []string
			executeServiceCommand = func(name string, args ...string) error {
				commands = append(commands, name+" "+strings.Join(args, " "))
				return nil
			}
			servicePrintln = func(message string) {
				messages = append(messages, message)
			}

			tmp := t.TempDir()
			code := installLaunchd(
				serviceArgs{
					Action:      actionInstall,
					HomeDir:     filepath.Join(tmp, "home"),
					NoBootStart: tt.noBootStart,
				},
				BuildInfo{Version: "test"},
				filepath.Join(tmp, "com.kdlbs.kandev.plist"),
				"gui/501/com.kdlbs.kandev",
				"gui/501",
			)
			if code != 0 {
				t.Fatalf("installLaunchd() = %d, want 0", code)
			}
			if !slices.Equal(messages, []string{tt.wantMessage}) {
				t.Fatalf("messages = %v, want %q", messages, tt.wantMessage)
			}
			gotKickstart := slices.Contains(commands, "launchctl kickstart gui/501/com.kdlbs.kandev")
			if gotKickstart != tt.wantKickstart {
				t.Fatalf("kickstart command presence = %v, want %v; commands=%v", gotKickstart, tt.wantKickstart, commands)
			}
		})
	}
}

func TestBuildJournalArgs(t *testing.T) {
	tests := []struct {
		name string
		args serviceArgs
		want []string
	}{
		{
			name: "user logs",
			args: serviceArgs{Action: actionLogs},
			want: []string{"--user-unit", "kandev.service", "-n", "200", "--no-pager"},
		},
		{
			name: "user logs follow keeps line count",
			args: serviceArgs{Action: actionLogs, Follow: true},
			want: []string{"--user-unit", "kandev.service", "-n", "200", "-f"},
		},
		{
			name: "system logs",
			args: serviceArgs{Action: actionLogs, System: true},
			want: []string{"-u", "kandev.service", "-n", "200", "--no-pager"},
		},
		{
			name: "system logs follow keeps line count",
			args: serviceArgs{Action: actionLogs, System: true, Follow: true},
			want: []string{"-u", "kandev.service", "-n", "200", "-f"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildJournalArgs(tt.args)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("buildJournalArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQuoteForUnitEscapesSystemdSpecials(t *testing.T) {
	got := serviceEnvLine("KANDEV_HOME_DIR", `/tmp/$USER/kandev%data`)
	want := `Environment="KANDEV_HOME_DIR=/tmp/$$USER/kandev%%data"`
	if got != want {
		t.Fatalf("serviceEnvLine() = %q, want %q", got, want)
	}
}

func TestServiceEnvLineAllowSpecifiersPreservesSystemdSpecifiers(t *testing.T) {
	got := serviceEnvLineAllowSpecifiers("PATH", `%h/.local/bin:$PATH`)
	want := `Environment="PATH=%h/.local/bin:$$PATH"`
	if got != want {
		t.Fatalf("serviceEnvLineAllowSpecifiers() = %q, want %q", got, want)
	}
}

func TestServicePathWithPrefixesPrependsValidUniqueDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("service PATH rendering is POSIX-only")
	}
	nodeBin := "/home/alice/.nvm/versions/node/v25.2.1/bin"
	got := servicePathWithPrefixes(
		"/usr/bin:/bin:%h/.local/bin",
		[]string{nodeBin, "/usr/bin", "relative/bin", "/bad:dir", "/bad%dir", "/bad\nline", "/bad\rline", nodeBin},
	)
	want := nodeBin + ":/usr/bin:/bin:%h/.local/bin"
	if got != want {
		t.Fatalf("servicePathWithPrefixes() = %q, want %q", got, want)
	}
}

func TestServiceNodeToolBinDirsSkipsTransientNpxSandbox(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("service PATH rendering is POSIX-only")
	}
	tmp := t.TempDir()
	nodeBin := filepath.Join(tmp, "home", "alice", ".nvm", "versions", "node", "v25.2.1", "bin")
	npxBin := filepath.Join(tmp, "home", "alice", ".npm", "_npx", "abc", "node_modules", ".bin")
	for _, dir := range []string{nodeBin, npxBin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"node", "npm"} {
		if err := os.WriteFile(filepath.Join(nodeBin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(npxBin, "npx"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", npxBin+string(os.PathListSeparator)+nodeBin)
	t.Setenv("npm_node_execpath", filepath.Join(nodeBin, "node"))

	got := serviceNodeToolBinDirs()
	if !slices.Contains(got, nodeBin) {
		t.Fatalf("serviceNodeToolBinDirs() = %v, want stable node bin %q", got, nodeBin)
	}
	if slices.Contains(got, npxBin) {
		t.Fatalf("serviceNodeToolBinDirs() included transient npx bin %q: %v", npxBin, got)
	}
}

func TestServiceNodeToolBinDirsResolvesFnmMultishellSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("service PATH rendering is POSIX-only")
	}
	tmp := t.TempDir()
	stableBin := filepath.Join(tmp, "home", "alice", ".local", "share", "fnm", "node-versions", "v25.2.1", "installation", "bin")
	multishellBin := filepath.Join(tmp, "run", "user", "1000", "fnm_multishells", "12345_67890", "bin")
	for _, dir := range []string{stableBin, multishellBin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"node", "npm", "npx"} {
		stablePath := filepath.Join(stableBin, name)
		if err := os.WriteFile(stablePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(stablePath, filepath.Join(multishellBin, name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", multishellBin)
	t.Setenv("npm_node_execpath", filepath.Join(multishellBin, "node"))

	got := serviceNodeToolBinDirs()
	if !slices.Contains(got, stableBin) {
		t.Fatalf("serviceNodeToolBinDirs() = %v, want stable fnm node bin %q", got, stableBin)
	}
	if slices.Contains(got, multishellBin) {
		t.Fatalf("serviceNodeToolBinDirs() included transient fnm multishell bin %q: %v", multishellBin, got)
	}
}

func TestBackupUnmanagedServiceFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kandev.service")
	original := "custom unit"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := backupUnmanagedServiceFile(path); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("backup = %q, want %q", data, original)
	}
}

func TestBackupUnmanagedServiceFileSkipsManagedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kandev.service")
	if err := os.WriteFile(path, []byte("# managed by kandev\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := backupUnmanagedServiceFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("managed file should not create backup, stat err=%v", err)
	}
}
