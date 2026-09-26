package launcher

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServiceBootstrapHomeResolutionPrecedence(t *testing.T) {
	flagHome := filepath.Join(t.TempDir(), "flag-home")
	envHome := filepath.Join(t.TempDir(), "env-home")

	tests := []struct {
		name string
		args serviceArgs
		env  string
		want string
	}{
		{name: "flag", args: serviceArgs{HomeDir: flagHome}, env: envHome, want: flagHome},
		{name: "environment", args: serviceArgs{}, env: envHome, want: envHome},
		{name: "system default", args: serviceArgs{System: true}, want: "/var/lib/kandev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KANDEV_HOME_DIR", tt.env)
			if got := serviceBootstrapHomeDir(tt.args); got != tt.want {
				t.Fatalf("serviceBootstrapHomeDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInstallSystemdDiscoversFlagHomeConfiguration(t *testing.T) {
	t.Setenv("KANDEV_HOME_DIR", "")
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "working"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(tmp, "working"))
	homeDir := filepath.Join(tmp, "flag-home")
	configPath := filepath.Join(homeDir, "config.yaml")
	writeServiceConfiguration(t, configPath, "server:\n  port: 40131\n")

	startup, err := loadServiceBootstrapConfig(serviceArgs{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("loadServiceBootstrapConfig() = %v", err)
	}
	if startup.Source.FilePath != configPath {
		t.Fatalf("selected config = %q, want %q", startup.Source.FilePath, configPath)
	}

	restoreServiceInstallHooks(t)
	executablePath = func() (string, error) { return "/opt/kandev/bin/kandev", nil }
	executeServiceCommand = func(string, ...string) error { return nil }
	servicePrintln = func(string) {}

	unitPath := filepath.Join(tmp, "kandev.service")
	code := installSystemd(serviceArgs{
		Action:      actionInstall,
		HomeDir:     homeDir,
		NoBootStart: true,
		Startup:     startup,
	}, BuildInfo{Version: "test"}, unitPath)
	if code != 0 {
		t.Fatalf("installSystemd() = %d, want 0", code)
	}
	unit, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	unitText := string(unit)
	if !strings.Contains(unitText, "Environment=KANDEV_INTERNAL_CONFIG_FILE="+configPath) {
		t.Fatalf("unit did not pin the flag-selected config file:\n%s", unitText)
	}
}

func TestInstallSystemdDiscoversSystemHomeConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("system service installation is unsupported on Windows")
	}
	t.Setenv("KANDEV_HOME_DIR", "")
	tmp := t.TempDir()
	workingDir := filepath.Join(tmp, "working")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workingDir)
	homeDir := filepath.Join(tmp, "system-home")
	configPath := filepath.Join(homeDir, "config.yaml")
	writeServiceConfiguration(t, configPath, "server:\n  port: 40132\n")
	currentUser, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}

	startup, err := loadServiceBootstrapConfig(serviceArgs{System: true, HomeDir: homeDir})
	if err != nil {
		t.Fatalf("loadServiceBootstrapConfig() = %v", err)
	}
	if startup.Source.FilePath != configPath {
		t.Fatalf("selected config = %q, want %q", startup.Source.FilePath, configPath)
	}

	restoreServiceInstallHooks(t)
	executablePath = func() (string, error) { return "/opt/kandev/bin/kandev", nil }
	executeServiceCommand = func(string, ...string) error { return nil }
	servicePrintln = func(string) {}

	unitPath := filepath.Join(tmp, "kandev.service")
	code := installSystemd(serviceArgs{
		Action:      actionInstall,
		System:      true,
		RunAs:       currentUser.Username,
		HomeDir:     homeDir,
		NoBootStart: true,
		Startup:     startup,
	}, BuildInfo{Version: "test"}, unitPath)
	if code != 0 {
		t.Fatalf("installSystemd() = %d, want 0", code)
	}
	unit, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	unitText := string(unit)
	if !strings.Contains(unitText, "Environment=KANDEV_INTERNAL_CONFIG_FILE="+configPath) {
		t.Fatalf("system unit did not pin the system-selected config file:\n%s", unitText)
	}
	if !strings.Contains(unitText, "Environment=KANDEV_SERVICE_MODE=system") {
		t.Fatalf("system unit did not retain system mode:\n%s", unitText)
	}
}

func TestServiceInstallHomeSourceWinsOverYAMLForBothManagers(t *testing.T) {
	tests := []struct {
		name string
		kind string
	}{
		{name: "systemd", kind: nativeServiceManagerSystemd},
		{name: "launchd", kind: nativeServiceManagerLaunchd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KANDEV_HOME_DIR", "")
			tmp := t.TempDir()
			workingDir := filepath.Join(tmp, "working")
			if err := os.MkdirAll(workingDir, 0o700); err != nil {
				t.Fatal(err)
			}
			t.Chdir(workingDir)
			writeServiceConfiguration(t, filepath.Join(workingDir, "config.yaml"), "homeDir: /yaml-home\n")
			flagHome := filepath.Join(tmp, "flag-home")
			startup, err := loadServiceBootstrapConfig(serviceArgs{HomeDir: flagHome})
			if err != nil {
				t.Fatalf("loadServiceBootstrapConfig() = %v", err)
			}

			restoreServiceInstallHooks(t)
			executablePath = func() (string, error) { return "/opt/kandev/bin/kandev", nil }
			executeServiceCommand = func(name string, args ...string) error {
				if tt.kind == nativeServiceManagerLaunchd && name == "launchctl" && len(args) > 0 && args[0] == "print" {
					return errors.New("not loaded")
				}
				return nil
			}
			servicePrintln = func(string) {}

			output := installServiceUnitForTest(t, tt.kind, tmp, serviceArgs{Action: actionInstall, HomeDir: flagHome, NoBootStart: true, Startup: startup})
			if !strings.Contains(output, expectedServiceHomeEntry(tt.kind, flagHome)) {
				t.Fatalf("%s service definition omitted flag home:\n%s", tt.name, output)
			}
			if strings.Contains(output, "yaml-home") {
				t.Fatalf("service metadata used YAML home despite the flag:\n%s", output)
			}
		})
	}
}

func TestServiceInstallEnvironmentHomeWinsOverYAMLForBothManagers(t *testing.T) {
	tests := []struct {
		name string
		kind string
	}{
		{name: "systemd", kind: nativeServiceManagerSystemd},
		{name: "launchd", kind: nativeServiceManagerLaunchd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			workingDir := filepath.Join(tmp, "working")
			if err := os.MkdirAll(workingDir, 0o700); err != nil {
				t.Fatal(err)
			}
			t.Chdir(workingDir)
			envHome := filepath.Join(tmp, "env-home")
			t.Setenv("KANDEV_HOME_DIR", envHome)
			writeServiceConfiguration(t, filepath.Join(workingDir, "config.yaml"), "homeDir: /yaml-home\n")
			startup, err := loadServiceBootstrapConfig(serviceArgs{})
			if err != nil {
				t.Fatalf("loadServiceBootstrapConfig() = %v", err)
			}

			restoreServiceInstallHooks(t)
			executablePath = func() (string, error) { return "/opt/kandev/bin/kandev", nil }
			executeServiceCommand = func(name string, args ...string) error {
				if tt.kind == nativeServiceManagerLaunchd && name == "launchctl" && len(args) > 0 && args[0] == "print" {
					return errors.New("not loaded")
				}
				return nil
			}
			servicePrintln = func(string) {}

			output := installServiceUnitForTest(t, tt.kind, tmp, serviceArgs{Action: actionInstall, NoBootStart: true, Startup: startup})
			if !strings.Contains(output, expectedServiceHomeEntry(tt.kind, envHome)) {
				t.Fatalf("%s service definition omitted environment home:\n%s", tt.name, output)
			}
			if strings.Contains(output, "yaml-home") {
				t.Fatalf("service metadata used YAML home despite the environment:\n%s", output)
			}
		})
	}
}

func installServiceUnitForTest(t *testing.T, kind, tmp string, args serviceArgs) string {
	t.Helper()
	switch kind {
	case nativeServiceManagerSystemd:
		path := filepath.Join(tmp, "kandev.service")
		if code := installSystemd(args, BuildInfo{}, path); code != 0 {
			t.Fatalf("installSystemd() = %d, want 0", code)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	case nativeServiceManagerLaunchd:
		path := filepath.Join(tmp, "com.kdlbs.kandev.plist")
		if code := installLaunchd(args, BuildInfo{}, path, "gui/501/com.kdlbs.kandev", "gui/501"); code != 0 {
			t.Fatalf("installLaunchd() = %d, want 0", code)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	default:
		t.Fatalf("unsupported test service manager %q", kind)
		return ""
	}
}

func expectedServiceHomeEntry(kind, homeDir string) string {
	if kind == nativeServiceManagerSystemd {
		return "Environment=KANDEV_HOME_DIR=" + homeDir
	}
	return "<key>KANDEV_HOME_DIR</key>\n      <string>" + homeDir + "</string>"
}

func restoreServiceInstallHooks(t *testing.T) {
	t.Helper()
	originalExecutablePath := executablePath
	originalExecuteServiceCommand := executeServiceCommand
	originalServicePrintln := servicePrintln
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		executeServiceCommand = originalExecuteServiceCommand
		servicePrintln = originalServicePrintln
	})
}

func writeServiceConfiguration(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
