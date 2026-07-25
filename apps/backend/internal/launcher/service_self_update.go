package launcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type nativeUpdateIntent struct {
	Version       int                 `json:"version"`
	TargetTag     string              `json:"target_tag"`
	TargetVersion string              `json:"target_version"`
	Install       nativeUpdateInstall `json:"install"`
}

type nativeUpdateInstall struct {
	Manager      string `json:"manager"`
	Mode         string `json:"mode"`
	Kind         string `json:"kind"`
	HomeDir      string `json:"home_dir"`
	LauncherPath string `json:"launcher_path"`
	Port         int    `json:"port,omitempty"`
	NoBootStart  bool   `json:"no_boot_start,omitempty"`
}

type nativeUpdateCommand struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func runNativeSelfUpdate(args serviceArgs) error {
	intent, err := readNativeUpdateIntent(args.Intent)
	if err != nil {
		return err
	}
	commands, err := planNativeSelfUpdate(intent)
	if err != nil {
		return err
	}
	if args.DryRun || os.Getenv("KANDEV_E2E_MOCK") == "true" {
		return printNativeUpdatePlan(intent, commands, args.DryRun)
	}
	return withoutStaleLauncherEnvironment(func() error {
		for _, step := range commands {
			if err := runCommand(step.Command, step.Args...); err != nil {
				return fmt.Errorf("%s failed: %w", formatNativeUpdateCommand(step), err)
			}
		}
		return nil
	})
}

func withoutStaleLauncherEnvironment(run func() error) (resultErr error) {
	const versionKey = "KANDEV_VERSION"
	const bundleKey = "KANDEV_BUNDLE_DIR"
	saved := []savedEnvironmentVariable{
		captureEnvironmentVariable(versionKey),
		captureEnvironmentVariable(bundleKey),
	}
	for _, variable := range saved {
		if err := os.Unsetenv(variable.Name); err != nil {
			_ = restoreEnvironmentVariables(saved)
			return fmt.Errorf("clear stale %s: %w", variable.Name, err)
		}
	}
	defer func() {
		if err := restoreEnvironmentVariables(saved); resultErr == nil && err != nil {
			resultErr = err
		}
	}()
	return run()
}

type savedEnvironmentVariable struct {
	Name    string
	Value   string
	Present bool
}

func captureEnvironmentVariable(name string) savedEnvironmentVariable {
	value, present := os.LookupEnv(name)
	return savedEnvironmentVariable{Name: name, Value: value, Present: present}
}

func restoreEnvironmentVariables(saved []savedEnvironmentVariable) error {
	for _, variable := range saved {
		if variable.Present {
			if err := os.Setenv(variable.Name, variable.Value); err != nil {
				return fmt.Errorf("restore %s: %w", variable.Name, err)
			}
			continue
		}
		if err := os.Unsetenv(variable.Name); err != nil {
			return fmt.Errorf("restore %s: %w", variable.Name, err)
		}
	}
	return nil
}

func readNativeUpdateIntent(path string) (nativeUpdateIntent, error) {
	if strings.TrimSpace(path) == "" {
		return nativeUpdateIntent{}, errors.New("kandev service self-update requires --intent <path>")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nativeUpdateIntent{}, fmt.Errorf("read self-update intent: %w", err)
	}
	var intent nativeUpdateIntent
	if err := json.Unmarshal(data, &intent); err != nil {
		return nativeUpdateIntent{}, fmt.Errorf("parse self-update intent: %w", err)
	}
	if intent.Version != 1 {
		return nativeUpdateIntent{}, fmt.Errorf("unsupported self-update intent version %d", intent.Version)
	}
	return intent, nil
}

func planNativeSelfUpdate(intent nativeUpdateIntent) ([]nativeUpdateCommand, error) {
	install := intent.Install
	if err := validateNativeUpdateInstall(install); err != nil {
		return nil, err
	}
	target := strings.TrimPrefix(intent.TargetVersion, "v")
	if target == "" {
		target = strings.TrimPrefix(intent.TargetTag, "v")
	}
	if target == "" {
		target = "latest"
	}
	installArgs := nativeServiceInstallArgs(install)
	var commands []nativeUpdateCommand
	switch install.Kind {
	case nativeInstallKindHomebrew:
		commands = append(commands,
			nativeUpdateCommand{Command: "brew", Args: []string{"upgrade", "kandev"}},
			nativeUpdateCommand{Command: "kandev", Args: installArgs},
		)
	case nativeInstallKindNPM:
		npmArgs := []string{"install", "-g"}
		if prefix := nativeNPMPrefix(install.LauncherPath); prefix != "" {
			npmArgs = append(npmArgs, "--prefix", prefix)
		}
		npmArgs = append(npmArgs, "kandev@"+target)
		commands = append(commands,
			nativeUpdateCommand{Command: "npm", Args: npmArgs},
			nativeUpdateCommand{Command: install.LauncherPath, Args: installArgs},
		)
	case nativeInstallKindNPX:
		npxArgs := append([]string{"-y", "kandev@" + target}, installArgs...)
		commands = append(commands, nativeUpdateCommand{Command: "npx", Args: npxArgs})
	default:
		return nil, fmt.Errorf("unsupported install kind %q", install.Kind)
	}
	restart, err := nativeServiceRestartCommand(install)
	if err != nil {
		return nil, err
	}
	return append(commands, restart), nil
}

func validateNativeUpdateInstall(install nativeUpdateInstall) error {
	if install.Mode != nativeServiceModeUser && install.Mode != nativeServiceModeSystem {
		return fmt.Errorf("unsupported service mode %q", install.Mode)
	}
	if install.Manager != nativeServiceManagerSystemd && install.Manager != nativeServiceManagerLaunchd {
		return fmt.Errorf("unsupported service manager %q", install.Manager)
	}
	if !filepath.IsAbs(install.HomeDir) {
		return errors.New("service metadata home_dir must be absolute")
	}
	if !filepath.IsAbs(install.LauncherPath) {
		return errors.New("service metadata launcher_path must be absolute")
	}
	return nil
}

func nativeServiceInstallArgs(install nativeUpdateInstall) []string {
	args := []string{"service", "install"}
	if install.Mode == nativeServiceModeSystem {
		args = append(args, "--system")
	}
	args = append(args, "--home-dir", install.HomeDir)
	if install.Port != 0 {
		args = append(args, "--port", strconv.Itoa(install.Port))
	}
	if install.NoBootStart {
		args = append(args, "--no-boot-start")
	}
	return args
}

func nativeServiceRestartCommand(install nativeUpdateInstall) (nativeUpdateCommand, error) {
	switch install.Manager {
	case nativeServiceManagerSystemd:
		args := []string{"restart", serviceUnitName}
		if install.Mode == nativeServiceModeUser {
			args = append([]string{"--user"}, args...)
		}
		return nativeUpdateCommand{Command: "systemctl", Args: args}, nil
	case nativeServiceManagerLaunchd:
		domain := "system"
		if install.Mode == nativeServiceModeUser {
			domain = "gui/" + strconv.Itoa(os.Getuid())
		}
		return nativeUpdateCommand{
			Command: "launchctl",
			Args:    []string{"kickstart", "-k", domain + "/com.kdlbs.kandev"},
		}, nil
	default:
		return nativeUpdateCommand{}, fmt.Errorf("unsupported service manager %q", install.Manager)
	}
}

func nativeNPMPrefix(launcherPath string) string {
	slashPath := filepath.ToSlash(launcherPath)
	const marker = "/lib/node_modules/"
	index := strings.Index(slashPath, marker)
	if index < 0 {
		return ""
	}
	if index == 0 {
		return string(filepath.Separator)
	}
	return filepath.FromSlash(slashPath[:index])
}

func printNativeUpdatePlan(intent nativeUpdateIntent, commands []nativeUpdateCommand, dryRun bool) error {
	out := struct {
		DryRun        bool                  `json:"dry_run"`
		Fake          bool                  `json:"fake"`
		TargetVersion string                `json:"target_version"`
		Commands      []nativeUpdateCommand `json:"commands"`
	}{
		DryRun:        dryRun,
		Fake:          os.Getenv("KANDEV_E2E_MOCK") == "true",
		TargetVersion: intent.TargetVersion,
		Commands:      commands,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func formatNativeUpdateCommand(command nativeUpdateCommand) string {
	return strings.Join(append([]string{command.Command}, command.Args...), " ")
}
