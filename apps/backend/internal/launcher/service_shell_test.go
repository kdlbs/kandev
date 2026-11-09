package launcher

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestServiceShellSelection(t *testing.T) {
	tests := []struct {
		name      string
		argv      []string
		wantShell string
		wantSet   bool
		wantError bool
	}{
		{name: "separate value", argv: []string{"install", "--shell", "/bin/zsh"}, wantShell: "/bin/zsh", wantSet: true},
		{name: "equals value", argv: []string{"install", "--shell=bash"}, wantShell: "bash", wantSet: true},
		{name: "reset", argv: []string{"install", "--shell=none"}, wantShell: "none", wantSet: true},
		{name: "omitted", argv: []string{"install"}},
		{name: "empty equals value", argv: []string{"install", "--shell="}, wantError: true},
		{name: "missing value", argv: []string{"install", "--shell"}, wantError: true},
		{name: "other action", argv: []string{"start", "--shell", "zsh"}, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseServiceArgs(tc.argv)
			if tc.wantError {
				if err == nil {
					t.Fatal("parseServiceArgs() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseServiceArgs(): %v", err)
			}
			if got.Shell != tc.wantShell || got.ShellSet != tc.wantSet {
				t.Fatalf("shell selection = (%q, %v), want (%q, %v)", got.Shell, got.ShellSet, tc.wantShell, tc.wantSet)
			}
		})
	}
}

func TestServiceShellRejectedBeforeMutationOnLinux(t *testing.T) {
	if runtime.GOOS != goosLinux {
		t.Skip("Linux service mutation guard")
	}
	temp := t.TempDir()
	t.Setenv("HOME", filepath.Join(temp, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(temp, "config"))
	t.Setenv("KANDEV_HOME_DIR", filepath.Join(temp, "data"))
	if err := os.MkdirAll(os.Getenv("HOME"), 0o700); err != nil {
		t.Fatal(err)
	}

	oldExecutablePath := executablePath
	oldExecuteServiceCommand := executeServiceCommand
	oldServicePrintln := servicePrintln
	oldLaunchctlCommand := launchctlCommand
	commandCalls := 0
	t.Cleanup(func() {
		executablePath = oldExecutablePath
		executeServiceCommand = oldExecuteServiceCommand
		servicePrintln = oldServicePrintln
		launchctlCommand = oldLaunchctlCommand
	})
	executablePath = func() (string, error) { return filepath.Join(temp, "bin", "kandev"), nil }
	executeServiceCommand = func(string, ...string) error {
		commandCalls++
		return nil
	}
	launchctlCommand = func(...string) error {
		commandCalls++
		return nil
	}
	servicePrintln = func(string) {}

	if code := runService([]string{"install", "--shell", "zsh", "--no-boot-start"}, BuildInfo{}); code == 0 {
		t.Fatal("runService() = 0, want non-Darwin shell selection to fail")
	}
	unitPath := linuxUnitPath(false)
	if _, err := os.Stat(unitPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("service definition exists after rejected shell selection: stat err=%v", err)
	}
	if _, err := os.Stat(nativeServiceMetadataPath(os.Getenv("KANDEV_HOME_DIR"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("service metadata exists after rejected shell selection: stat err=%v", err)
	}
	if commandCalls != 0 {
		t.Fatalf("service command calls = %d, want zero for rejected shell selection", commandCalls)
	}
}

func TestNativeSelfUpdatePreservesServiceShell(t *testing.T) {
	tests := []struct {
		name       string
		shellField string
		want       string
	}{
		{name: "selected", shellField: `,"shell":"/bin/zsh"`, want: "--shell\x00/bin/zsh"},
		{name: "legacy missing", want: "--shell=none"},
		{name: "legacy null", shellField: `,"shell":null`, want: "--shell=none"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			intentPath := filepath.Join(t.TempDir(), "intent.json")
			intentJSON := `{"version":1,"target_version":"1.2.3","install":{"manager":"launchd","mode":"user","kind":"npm","home_dir":"/Users/alice/.kandev","launcher_path":"/opt/homebrew/lib/node_modules/kandev/bin/kandev"` + tc.shellField + `}}`
			if err := os.WriteFile(intentPath, []byte(intentJSON), 0o600); err != nil {
				t.Fatal(err)
			}
			intent, err := readNativeUpdateIntent(intentPath)
			if err != nil {
				t.Fatalf("read update intent: %v", err)
			}
			commands, err := planNativeSelfUpdate(intent)
			if err != nil {
				t.Fatalf("plan update: %v", err)
			}
			joined := strings.Join(commands[1].Args, "\x00")
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("install args = %q, want %q", joined, tc.want)
			}
		})
	}
}

func TestNativeSelfUpdatePreservesShellForEveryPackageManager(t *testing.T) {
	tests := []struct {
		kind        string
		launcher    string
		installTool string
	}{
		{kind: nativeInstallKindHomebrew, launcher: "/opt/homebrew/Cellar/kandev/1.2.3/bin/kandev", installTool: "kandev"},
		{kind: nativeInstallKindNPM, launcher: "/opt/node_modules/kandev/bin/kandev", installTool: "/opt/node_modules/kandev/bin/kandev"},
		{kind: nativeInstallKindNPX, launcher: "/Users/alice/.npm/_npx/hash/node_modules/kandev/bin/kandev", installTool: "npx"},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			commands, err := planNativeSelfUpdate(nativeUpdateIntent{
				TargetVersion: "1.2.4",
				Install: nativeUpdateInstall{
					Manager: nativeServiceManagerLaunchd, Mode: nativeServiceModeUser,
					Kind: tc.kind, HomeDir: "/Users/alice/.kandev", LauncherPath: tc.launcher,
					Shell: "/Users/alice/.local/bin/zsh",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, command := range commands {
				if command.Command != tc.installTool {
					continue
				}
				if !hasAdjacentValues(command.Args, "--shell", "/Users/alice/.local/bin/zsh") {
					t.Fatalf("%s install args = %#v, want persisted shell", tc.kind, command.Args)
				}
				return
			}
			t.Fatalf("no %s install command in %#v", tc.kind, commands)
		})
	}
	if args := nativeServiceInstallArgs(nativeUpdateInstall{Manager: nativeServiceManagerSystemd, Mode: nativeServiceModeUser}); hasArgument(args, "--shell=none") {
		t.Fatalf("systemd install args contain a macOS shell option: %#v", args)
	}
}

func TestRenderLaunchdShellArgumentsRemainStructured(t *testing.T) {
	executable := `/Applications/Kandev & Tools/kandev`
	shell := `/Users/alice/bin/zsh's $shell`
	plist := renderLaunchdPlist(nativeServiceUnitInput{
		Executable: executable,
		HomeDir:    "/Users/alice/.kandev",
		LogDir:     "/Users/alice/.kandev/logs",
		Shell:      shell,
	})
	arguments, err := parseLaunchdProgramArguments([]byte(plist))
	if err != nil {
		t.Fatalf("parse rendered ProgramArguments: %v", err)
	}
	want := []string{executable, "service", "run", "--shell", shell}
	if strings.Join(arguments, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("ProgramArguments = %#v, want %#v", arguments, want)
	}
}

func TestResolveServiceShell(t *testing.T) {
	temp := t.TempDir()
	binDir := filepath.Join(temp, "shells")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	zsh := filepath.Join(binDir, "zsh")
	writeServiceShellFixture(t, zsh, "#!/bin/sh\nexit 0\n", 0o755)
	t.Setenv("PATH", binDir)

	resolved, err := resolveServiceShell("zsh")
	if err != nil {
		t.Fatalf("resolveServiceShell(zsh): %v", err)
	}
	if resolved != zsh {
		t.Fatalf("resolved shell = %q, want %q", resolved, zsh)
	}

	specialDir := filepath.Join(temp, `Kandev & Tools '$data'`)
	if err := os.MkdirAll(specialDir, 0o700); err != nil {
		t.Fatal(err)
	}
	specialShell := filepath.Join(specialDir, "bash")
	writeServiceShellFixture(t, specialShell, "#!/bin/sh\nexit 0\n", 0o755)
	if got, err := resolveServiceShell(specialShell); err != nil || got != specialShell {
		t.Fatalf("resolve special shell = (%q, %v), want (%q, nil)", got, err, specialShell)
	}

	noExec := filepath.Join(binDir, "bash")
	writeServiceShellFixture(t, noExec, "#!/bin/sh\nexit 0\n", 0o644)
	for _, value := range []string{"", "fish", "relative/zsh", filepath.Join(binDir, "missing-zsh"), noExec, "/bin/bash\n"} {
		if _, err := resolveServiceShell(value); err == nil {
			t.Errorf("resolveServiceShell(%q) error = nil, want error", value)
		}
	}
}

func TestSelectManagedLaunchdShell(t *testing.T) {
	temp := t.TempDir()
	homeDir := filepath.Join(temp, "home")
	servicePath := filepath.Join(temp, "LaunchAgents", "com.kdlbs.kandev.plist")
	bash := filepath.Join(temp, "shells", "bash")
	zsh := filepath.Join(temp, "shells", "zsh")
	writeServiceShellFixture(t, bash, "#!/bin/sh\nexit 0\n", 0o755)
	writeServiceShellFixture(t, zsh, "#!/bin/sh\nexit 0\n", 0o755)

	writeManagedLaunchdShellFixture(t, servicePath, homeDir, zsh, zsh)
	args := serviceArgs{Action: actionInstall}
	if got, err := selectManagedLaunchdShell(args, servicePath, homeDir); err != nil || got != zsh {
		t.Fatalf("preserved shell = (%q, %v), want (%q, nil)", got, err, zsh)
	}
	args.ShellSet, args.Shell = true, "none"
	if got, err := selectManagedLaunchdShell(args, servicePath, homeDir); err != nil || got != "" {
		t.Fatalf("reset shell = (%q, %v), want empty", got, err)
	}
	args.Shell = bash
	if got, err := selectManagedLaunchdShell(args, servicePath, homeDir); err != nil || got != bash {
		t.Fatalf("explicit repair = (%q, %v), want (%q, nil)", got, err, bash)
	}

	writeManagedLaunchdShellFixture(t, servicePath, homeDir, zsh, bash)
	args = serviceArgs{Action: actionInstall}
	if _, err := selectManagedLaunchdShell(args, servicePath, homeDir); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("metadata mismatch error = %v, want actionable mismatch", err)
	}
	args.ShellSet, args.Shell = true, bash
	if got, err := selectManagedLaunchdShell(args, servicePath, homeDir); err != nil || got != bash {
		t.Fatalf("explicit metadata repair = (%q, %v), want (%q, nil)", got, err, bash)
	}

	writeManagedLaunchdShellFixture(t, servicePath, homeDir, filepath.Join(temp, "missing", "zsh"), filepath.Join(temp, "missing", "zsh"))
	args = serviceArgs{Action: actionInstall}
	if _, err := selectManagedLaunchdShell(args, servicePath, homeDir); err == nil || !strings.Contains(err.Error(), "--shell=none") {
		t.Fatalf("invalid saved shell error = %v, want repair guidance", err)
	}
	args.ShellSet, args.Shell = true, "none"
	if got, err := selectManagedLaunchdShell(args, servicePath, homeDir); err != nil || got != "" {
		t.Fatalf("invalid saved shell reset = (%q, %v), want empty", got, err)
	}

	if err := os.WriteFile(servicePath, []byte("<!-- managed by kandev --><plist><dict><key>ProgramArguments</key><array><string>/bin/kandev</string><string>service</string></array></dict></plist>"), 0o600); err != nil {
		t.Fatal(err)
	}
	args = serviceArgs{Action: actionInstall}
	if _, err := selectManagedLaunchdShell(args, servicePath, homeDir); err == nil || !strings.Contains(err.Error(), "not a recognized") {
		t.Fatalf("malformed managed invocation error = %v, want rejection", err)
	}
	args.ShellSet, args.Shell = true, serviceShellNone
	if got, err := selectManagedLaunchdShell(args, servicePath, homeDir); err != nil || got != "" {
		t.Fatalf("explicit reset of malformed managed invocation = (%q, %v), want empty shell", got, err)
	}
	symlinkPath := servicePath + ".link"
	if err := os.Symlink(servicePath, symlinkPath); err != nil {
		t.Fatalf("create service definition symlink: %v", err)
	}
	if _, err := selectManagedLaunchdShell(args, symlinkPath, homeDir); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("explicit reset through service symlink error = %v, want regular-file rejection", err)
	}
}

func TestInstallLaunchdMigratesIssueLoginShellWrapper(t *testing.T) {
	oldExecutablePath := executablePath
	oldExecuteServiceCommand := executeServiceCommand
	oldServicePrintln := servicePrintln
	oldLaunchctlCommand := launchctlCommand
	t.Cleanup(func() {
		executablePath = oldExecutablePath
		executeServiceCommand = oldExecuteServiceCommand
		servicePrintln = oldServicePrintln
		launchctlCommand = oldLaunchctlCommand
	})
	executable := "/opt/homebrew/bin/kandev"
	executablePath = func() (string, error) { return executable, nil }
	executeServiceCommand = func(string, ...string) error { return nil }
	servicePrintln = func(string) {}
	launchctlCommand = func(args ...string) error {
		if len(args) > 0 && args[0] == "print" {
			return errors.New("not loaded")
		}
		return nil
	}

	temp := t.TempDir()
	homeDir := filepath.Join(temp, "home")
	plistPath := filepath.Join(temp, "LaunchAgents", "com.kdlbs.kandev.plist")
	shellPath := filepath.Join(temp, "shells", serviceShellBash)
	writeServiceShellFixture(t, shellPath, "#!/bin/sh\nexit 0\n", 0o755)
	issueWrapper := `<!-- managed by kandev -->
<plist><dict><key>ProgramArguments</key><array>
<string>/bin/zsh</string><string>-l</string><string>-c</string>
<string>exec /usr/bin/env "$@"</string><string>kandev-service</string>
<string>/usr/local/bin/node</string><string>/usr/local/lib/node_modules/kandev/bin/cli.js</string><string>--headless</string>
</array></dict></plist>`
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plistPath, []byte(issueWrapper), 0o600); err != nil {
		t.Fatal(err)
	}
	install := func(shell string, shellSet bool, want []string) {
		t.Helper()
		args := serviceArgs{Action: actionInstall, HomeDir: homeDir, NoBootStart: true, Shell: shell, ShellSet: shellSet}
		if code := installLaunchd(args, BuildInfo{Version: "1.2.3"}, plistPath, "gui/501/com.kdlbs.kandev", "gui/501"); code != 0 {
			t.Fatalf("installLaunchd() = %d", code)
		}
		data, err := os.ReadFile(plistPath)
		if err != nil {
			t.Fatal(err)
		}
		arguments, err := parseLaunchdProgramArguments(data)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(arguments, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("ProgramArguments = %#v, want %#v", arguments, want)
		}
	}
	install(shellPath, true, []string{executable, "service", actionRun, "--shell", shellPath})
	writeServiceShellFixture(t, plistPath, issueWrapper, 0o600)
	install(serviceShellNone, true, []string{executable, "--headless"})
}

func TestSelectManagedLaunchdShellPreservesLegacyNodeInvocation(t *testing.T) {
	temp := t.TempDir()
	homeDir := filepath.Join(temp, "home")
	servicePath := filepath.Join(temp, "LaunchAgents", "com.kdlbs.kandev.plist")
	nodePath := "/usr/local/bin/node"
	cliEntry := "/usr/local/lib/node_modules/kandev/bin/cli.js"
	writeServiceShellFixture(t, servicePath, `<!-- managed by kandev -->
<plist><dict><key>ProgramArguments</key><array>
<string>`+nodePath+`</string><string>`+cliEntry+`</string><string>--headless</string>
</array></dict></plist>`, 0o600)
	metadata := map[string]any{
		"version": nativeServiceMetadataVersion, "manager": nativeServiceManagerLaunchd,
		"mode": nativeServiceModeUser, "home_dir": homeDir, "service_path": servicePath,
		"node_path": nodePath, "cli_entry": cliEntry,
	}
	metadataData, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(nativeServiceMetadataPath(homeDir)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nativeServiceMetadataPath(homeDir), metadataData, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := selectManagedLaunchdShell(serviceArgs{Action: actionInstall}, servicePath, homeDir); err != nil || got != "" {
		t.Fatalf("legacy direct service shell = (%q, %v), want empty", got, err)
	}
}

func TestInstallLaunchdRejectsMetadataIdentityMismatchesBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*nativeServiceMetadata)
	}{
		{name: "manager", mutate: func(metadata *nativeServiceMetadata) { metadata.Manager = nativeServiceManagerSystemd }},
		{name: "mode", mutate: func(metadata *nativeServiceMetadata) { metadata.Mode = nativeServiceModeSystem }},
		{name: "service path", mutate: func(metadata *nativeServiceMetadata) { metadata.ServicePath += ".other" }},
		{name: "home directory", mutate: func(metadata *nativeServiceMetadata) { metadata.HomeDir += ".other" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldExecutablePath := executablePath
			oldExecuteServiceCommand := executeServiceCommand
			oldServicePrintln := servicePrintln
			oldLaunchctlCommand := launchctlCommand
			t.Cleanup(func() {
				executablePath = oldExecutablePath
				executeServiceCommand = oldExecuteServiceCommand
				servicePrintln = oldServicePrintln
				launchctlCommand = oldLaunchctlCommand
			})
			executablePath = func() (string, error) { return "/opt/homebrew/bin/kandev", nil }
			servicePrintln = func(string) {}
			commandCalls := 0
			executeServiceCommand = func(string, ...string) error {
				commandCalls++
				return nil
			}
			launchctlCommand = func(...string) error {
				commandCalls++
				return nil
			}

			temp := t.TempDir()
			homeDir := filepath.Join(temp, "home")
			servicePath := filepath.Join(temp, "LaunchAgents", "com.kdlbs.kandev.plist")
			shellPath := filepath.Join(temp, "shells", "zsh")
			writeServiceShellFixture(t, shellPath, "#!/bin/sh\nexit 0\n", 0o755)
			writeManagedLaunchdShellFixture(t, servicePath, homeDir, shellPath, shellPath)
			metadataPath := nativeServiceMetadataPath(homeDir)
			metadataData, err := os.ReadFile(metadataPath)
			if err != nil {
				t.Fatal(err)
			}
			var metadata nativeServiceMetadata
			if err := json.Unmarshal(metadataData, &metadata); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&metadata)
			metadataData, err = json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(metadataPath, metadataData, 0o600); err != nil {
				t.Fatal(err)
			}
			plistBefore, err := os.ReadFile(servicePath)
			if err != nil {
				t.Fatal(err)
			}
			metadataBefore, err := os.ReadFile(metadataPath)
			if err != nil {
				t.Fatal(err)
			}

			args := serviceArgs{Action: actionInstall, HomeDir: homeDir, NoBootStart: true}
			code := installLaunchd(args, BuildInfo{Version: "1.2.3"}, servicePath, "gui/501/com.kdlbs.kandev", "gui/501")
			if code == 0 {
				t.Fatal("installLaunchd() = 0, want identity mismatch error")
			}
			if commandCalls != 0 {
				t.Fatalf("service command/reload calls = %d, want zero before repair", commandCalls)
			}
			plistAfter, err := os.ReadFile(servicePath)
			if err != nil {
				t.Fatal(err)
			}
			metadataAfter, err := os.ReadFile(metadataPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(plistAfter) != string(plistBefore) || string(metadataAfter) != string(metadataBefore) {
				t.Fatal("plist or metadata changed after identity mismatch")
			}
			for _, override := range []string{shellPath, serviceShellNone} {
				if _, err := selectManagedLaunchdShell(serviceArgs{
					Action: actionInstall, Shell: override, ShellSet: true,
				}, servicePath, homeDir); err != nil {
					t.Fatalf("explicit shell %q did not permit repair: %v", override, err)
				}
			}
		})
	}
}

func TestInstallLaunchdPersistsShellAcrossReinstallAndReset(t *testing.T) {
	oldExecutablePath := executablePath
	oldExecuteServiceCommand := executeServiceCommand
	oldServicePrintln := servicePrintln
	oldLaunchctlCommand := launchctlCommand
	t.Cleanup(func() {
		executablePath = oldExecutablePath
		executeServiceCommand = oldExecuteServiceCommand
		servicePrintln = oldServicePrintln
		launchctlCommand = oldLaunchctlCommand
	})
	executable := "/opt/homebrew/bin/kandev"
	executablePath = func() (string, error) { return executable, nil }
	executeServiceCommand = func(string, ...string) error { return nil }
	servicePrintln = func(string) {}
	launchctlCommand = func(args ...string) error {
		if len(args) > 0 && args[0] == "print" {
			return errors.New("not loaded")
		}
		return nil
	}

	temp := t.TempDir()
	homeDir := filepath.Join(temp, "home")
	plistPath := filepath.Join(temp, "LaunchAgents", "com.kdlbs.kandev.plist")
	shellPath := "/bin/bash"
	install := func(args serviceArgs) {
		t.Helper()
		args.Action = actionInstall
		args.HomeDir = homeDir
		args.NoBootStart = true
		if code := installLaunchd(args, BuildInfo{Version: "1.2.3"}, plistPath, "gui/501/com.kdlbs.kandev", "gui/501"); code != 0 {
			t.Fatalf("installLaunchd() = %d", code)
		}
	}
	assertShell := func(want string) {
		t.Helper()
		data, err := os.ReadFile(plistPath)
		if err != nil {
			t.Fatal(err)
		}
		arguments, err := parseLaunchdProgramArguments(data)
		if err != nil {
			t.Fatal(err)
		}
		metadataData, err := os.ReadFile(nativeServiceMetadataPath(homeDir))
		if err != nil {
			t.Fatal(err)
		}
		var metadata nativeServiceMetadata
		if err := json.Unmarshal(metadataData, &metadata); err != nil {
			t.Fatal(err)
		}
		if want == "" {
			if strings.Join(arguments, "\x00") != executable+"\x00--headless" || metadata.Shell != "" {
				t.Fatalf("reset arguments=%#v metadata shell=%q", arguments, metadata.Shell)
			}
			return
		}
		if strings.Join(arguments, "\x00") != executable+"\x00service\x00run\x00--shell\x00"+want || metadata.Shell != want {
			t.Fatalf("shell arguments=%#v metadata shell=%q, want %q", arguments, metadata.Shell, want)
		}
	}

	install(serviceArgs{Shell: shellPath, ShellSet: true})
	assertShell(shellPath)
	install(serviceArgs{})
	assertShell(shellPath)
	install(serviceArgs{Shell: "none", ShellSet: true})
	assertShell("")
}

func TestServiceShellRestoresEveryExplicitLaunchdValue(t *testing.T) {
	plist := renderLaunchdPlist(nativeServiceUnitInput{
		Executable: "/opt/kandev/bin/kandev", HomeDir: "/Users/alice/.kandev",
		LogDir: "/Users/alice/.kandev/logs", Port: 38429, BundleDir: "/opt/kandev/bundle",
		Version: "1.2.3", ConfigFile: "/Users/alice/config.yaml", ConfigHomeFile: true,
		LogLevel: "debug",
	})
	start := strings.Index(plist, "<key>EnvironmentVariables</key>")
	if start < 0 {
		t.Fatal("EnvironmentVariables dictionary is missing")
	}
	endOffset := strings.Index(plist[start:], "</dict>")
	if endOffset < 0 {
		t.Fatal("EnvironmentVariables dictionary is not closed")
	}
	environment := plist[start : start+endOffset]
	keys := regexp.MustCompile(`<key>([^<]+)</key>\s*<string>`).FindAllStringSubmatch(environment, -1)
	restored := serviceShellRestoreEnvironmentKeys()
	for _, match := range keys {
		key := match[1]
		if key == "PATH" {
			continue
		}
		if !hasArgument(restored, key) {
			t.Errorf("explicit launchd value %q is not restored after shell startup", key)
		}
	}
}

func TestServiceShellSubprocess(t *testing.T) {
	bash, err := resolveServiceShell("/bin/bash")
	if err != nil {
		t.Skipf("Bash is unavailable: %v", err)
	}
	accountHome := t.TempDir()
	shimDir := filepath.Join(accountHome, "tools with spaces")
	if err := os.MkdirAll(shimDir, 0o700); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(accountHome, "interactive-marker")
	profile := "export PATH=" + shellSingleQuote(shimDir) + ":$PATH\n" +
		"export KANDEV_LOGIN_MARKER='login value & $literal'\n" +
		"export KANDEV_SERVICE_MANAGER='shell override'\n"
	if err := os.WriteFile(filepath.Join(accountHome, ".bash_profile"), []byte(profile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(accountHome, ".bashrc"), []byte("touch \"$KANDEV_INTERACTIVE_MARKER\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(accountHome, "selected-env.txt")
	executable := filepath.Join(accountHome, `Kandev & '$special'`, "kandev")
	script := `#!/bin/sh
printf '%s\n' "$HOME" "$SHELL" "$KANDEV_SERVICE_MANAGER" "$KANDEV_HOME_DIR" "$KANDEV_SERVER_PORT" "$KANDEV_LOGIN_MARKER" "$PATH" "$#" "$1" > "$KANDEV_TEST_RESULT"
exit "$KANDEV_TEST_EXIT"
`
	writeServiceShellFixture(t, executable, script, 0o755)
	inherited := []string{
		"PATH=/usr/bin:/bin", "HOME=/wrong/home", "SHELL=/wrong/shell",
		"KANDEV_SERVICE_MANAGER=launchd", "KANDEV_HOME_DIR=/service home/&data",
		"KANDEV_SERVER_PORT=38429", "KANDEV_TEST_RESULT=" + resultPath,
		"KANDEV_TEST_EXIT=7", "KANDEV_INTERACTIVE_MARKER=" + markerPath,
		"API_TOKEN=test-secret",
	}
	invocation, err := buildServiceShellInvocation(bash, accountHome, executable, inherited)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(invocation.Path, invocation.Args[1:]...)
	cmd.Env = invocation.Env
	if err := cmd.Run(); err == nil {
		t.Fatal("shell subprocess exit = nil, want fixture status 7")
	} else {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
			t.Fatalf("shell subprocess exit = %v, want status 7", err)
		}
	}
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read child environment: %v", err)
	}
	values := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(values) != 9 {
		t.Fatalf("child recorded %d values, want 9: %q", len(values), values)
	}
	if values[0] != accountHome || values[1] != bash || values[2] != "launchd" ||
		values[3] != "/service home/&data" || values[4] != "38429" ||
		values[5] != "login value & $literal" || !strings.Contains(values[6], shimDir) ||
		values[7] != "1" || values[8] != "--headless" {
		t.Fatalf("unexpected child environment and argv: %#v", values)
	}
	if _, err := os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interactive startup marker exists: stat err=%v", err)
	}
	if strings.Contains(strings.Join(invocation.Args, "\x00"), "test-secret") {
		t.Fatal("an unrelated inherited value was copied to shell argv")
	}
}

func TestServiceShellInternalActionIsHiddenAndRestricted(t *testing.T) {
	args, err := parseServiceArgs([]string{"run", "--shell", "/bin/bash"})
	if err != nil {
		t.Fatal(err)
	}
	if args.Action != actionRun || !args.ShellSet {
		t.Fatalf("internal action args = %+v", args)
	}
	if strings.Contains(serviceHelp, "service run") {
		t.Fatal("internal service shell action appears in public service help")
	}
	if err := prepareServiceShellArgs(&args, goosLinux); err == nil {
		t.Fatal("Linux accepted the internal shell action")
	}
	args.System = true
	if err := prepareServiceShellArgs(&args, goosDarwin); err == nil {
		t.Fatal("system mode accepted the internal shell action")
	}
}

func writeManagedLaunchdShellFixture(t *testing.T, servicePath, homeDir, plistShell, metadataShell string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(nativeServiceMetadataPath(homeDir)), 0o700); err != nil {
		t.Fatal(err)
	}
	plist := renderLaunchdPlist(nativeServiceUnitInput{
		Executable: "/opt/kandev/bin/kandev",
		HomeDir:    homeDir,
		LogDir:     filepath.Join(homeDir, "logs"),
		Shell:      plistShell,
	})
	if err := os.WriteFile(servicePath, []byte(plist), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := nativeServiceMetadata{
		Version:     nativeServiceMetadataVersion,
		Manager:     nativeServiceManagerLaunchd,
		Mode:        nativeServiceModeUser,
		HomeDir:     homeDir,
		ServicePath: servicePath,
		Shell:       metadataShell,
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nativeServiceMetadataPath(homeDir), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeServiceShellFixture(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func hasAdjacentValues(values []string, first, second string) bool {
	for i := 0; i+1 < len(values); i++ {
		if values[i] == first && values[i+1] == second {
			return true
		}
	}
	return false
}

func hasArgument(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
