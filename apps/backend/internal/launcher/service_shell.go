package launcher

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"github.com/kandev/kandev/internal/common/config"
)

const (
	serviceShellCommand = `exec /usr/bin/env "$@"`
	serviceShellBash    = "bash"
	serviceShellZsh     = "zsh"
	serviceShellNone    = "none"
)

var lookupServiceShellUser = user.LookupId

type serviceShellInvocation struct {
	Path string
	Args []string
	Env  []string
}

func prepareServiceShellArgs(args *serviceArgs, goos string) error {
	if err := validateServiceShellAction(args); err != nil {
		return err
	}
	if args.Action != actionInstall && args.Action != actionRun {
		return nil
	}
	if err := validateServiceShellPlatform(args, goos); err != nil {
		return err
	}
	if args.Action == actionRun {
		return prepareNativeServiceShellArgs(args)
	}
	return prepareOptionalServiceShellArgs(args)
}

func validateServiceShellAction(args *serviceArgs) error {
	if args.Action != actionInstall && args.Action != actionRun && args.ShellSet {
		return errors.New("--shell is valid only for kandev service install")
	}
	return nil
}

func validateServiceShellPlatform(args *serviceArgs, goos string) error {
	if goos != goosDarwin && (args.ShellSet || args.Action == actionRun) {
		return errors.New("service login shells are supported only for macOS user services")
	}
	if args.System && (args.ShellSet || args.Action == actionRun) {
		return errors.New("service login shells are not supported for system services")
	}
	return nil
}

func prepareNativeServiceShellArgs(args *serviceArgs) error {
	if !args.ShellSet || args.Shell == serviceShellNone || !filepath.IsAbs(args.Shell) {
		return errors.New("internal service shell action requires --shell <absolute-path>")
	}
	resolved, err := validateServiceShellExecutable(args.Shell)
	if err != nil {
		return err
	}
	args.Shell = resolved
	return nil
}

func prepareOptionalServiceShellArgs(args *serviceArgs) error {
	if !args.ShellSet || args.Shell == serviceShellNone {
		return nil
	}
	resolved, err := resolveServiceShell(args.Shell)
	if err != nil {
		return err
	}
	args.Shell = resolved
	return nil
}

func resolveServiceShell(value string) (string, error) {
	if value == "" || hasServiceShellControl(value) {
		return "", errors.New("--shell requires bash, zsh, or an absolute executable path")
	}
	if filepath.IsAbs(value) {
		return validateServiceShellExecutable(value)
	}
	if strings.ContainsRune(value, filepath.Separator) {
		return "", errors.New("--shell path must be absolute")
	}
	if value != serviceShellBash && value != serviceShellZsh {
		return "", fmt.Errorf("unsupported service shell %q; choose bash or zsh", value)
	}
	resolved, err := exec.LookPath(value)
	if err != nil {
		return "", fmt.Errorf("resolve service shell %q from PATH: %w", value, err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve absolute service shell path: %w", err)
	}
	return validateServiceShellExecutable(absolute)
}

func validateServiceShellExecutable(value string) (string, error) {
	if !filepath.IsAbs(value) || hasServiceShellControl(value) {
		return "", errors.New("service shell path must be an absolute path without control characters")
	}
	clean := filepath.Clean(value)
	base := filepath.Base(clean)
	if base != serviceShellBash && base != serviceShellZsh {
		return "", fmt.Errorf("unsupported service shell %q; choose a Bash or Zsh executable", value)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("service shell %q is unavailable: %w", clean, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("service shell %q is not an executable file", clean)
	}
	return clean, nil
}

func hasServiceShellControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func selectManagedLaunchdShell(args serviceArgs, servicePath, homeDir string) (string, error) {
	if args.System {
		if args.ShellSet {
			return "", errors.New("service login shells are not supported for system services")
		}
		return "", nil
	}
	data, managed, err := readManagedLaunchdPlist(servicePath)
	if err != nil {
		return "", err
	}
	if args.ShellSet {
		if args.Shell == serviceShellNone {
			return "", nil
		}
		return validateServiceShellExecutable(args.Shell)
	}
	if !managed {
		return "", nil
	}
	metadata, exists, err := readNativeServiceShellMetadata(homeDir)
	if err != nil {
		return "", err
	}
	if exists && !nativeServiceShellMetadataMatches(metadata, servicePath, homeDir) {
		return "", fmt.Errorf(
			"service install metadata does not match managed definition %q; reinstall with --shell <bash|zsh|path> or --shell=none to repair",
			servicePath,
		)
	}
	savedShell, err := managedLaunchdShellArgument(data, metadata)
	if err != nil {
		return "", fmt.Errorf("managed launchd definition %q is invalid: %w", servicePath, err)
	}
	if savedShell != "" {
		resolved, err := validateServiceShellExecutable(savedShell)
		if err != nil {
			return "", serviceShellRepairError(err)
		}
		savedShell = resolved
	}
	if exists && metadata.Shell != savedShell {
		return "", fmt.Errorf(
			"saved service shell in %q does not match its install metadata; reinstall with --shell <bash|zsh|path> or --shell=none",
			servicePath,
		)
	}
	return savedShell, nil
}

func readManagedLaunchdPlist(servicePath string) ([]byte, bool, error) {
	info, err := os.Lstat(servicePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect existing service definition %q: %w", servicePath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("managed service definition %q must be a regular file", servicePath)
	}
	data, err := os.ReadFile(servicePath)
	if err != nil {
		return nil, false, fmt.Errorf("read existing service definition %q: %w", servicePath, err)
	}
	return data, bytes.Contains(data, []byte(managedMarker)), nil
}

func managedLaunchdShellArgument(data []byte, metadata nativeServiceMetadata) (string, error) {
	arguments, err := parseLaunchdProgramArguments(data)
	if err != nil {
		return "", err
	}
	if legacyLaunchdDirectInvocation(arguments, metadata.NodePath, metadata.CLIEntry) {
		return "", nil
	}
	if len(arguments) < 1 || !filepath.IsAbs(arguments[0]) || filepath.Base(arguments[0]) != "kandev" {
		return "", errors.New("ProgramArguments must start with the absolute kandev executable")
	}
	if len(arguments) == 2 && arguments[1] == "--headless" {
		return "", nil
	}
	if len(arguments) == 5 && arguments[1] == "service" && arguments[2] == actionRun && arguments[3] == "--shell" {
		return arguments[4], nil
	}
	return "", errors.New("ProgramArguments is not a recognized Kandev service invocation")
}

func legacyLaunchdDirectInvocation(arguments []string, nodePath, cliEntry string) bool {
	if nodePath == "" || cliEntry == "" || !filepath.IsAbs(nodePath) || !filepath.IsAbs(cliEntry) || len(arguments) < 2 {
		return false
	}
	if arguments[0] != nodePath || arguments[1] != cliEntry {
		return false
	}
	return len(arguments) == 2 || (len(arguments) == 3 && arguments[2] == "--headless")
}

func parseLaunchdProgramArguments(data []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var key string
	var arguments []string
	found := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse plist XML: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "key" {
			if err := decoder.DecodeElement(&key, &start); err != nil {
				return nil, fmt.Errorf("parse plist key: %w", err)
			}
			continue
		}
		if key != "ProgramArguments" {
			key = ""
			continue
		}
		if start.Name.Local != "array" || found {
			return nil, errors.New("ProgramArguments must be one array")
		}
		arguments, err = decodeLaunchdStringArray(decoder, start)
		if err != nil {
			return nil, fmt.Errorf("parse ProgramArguments: %w", err)
		}
		found = true
		key = ""
	}
	if !found || len(arguments) == 0 {
		return nil, errors.New("ProgramArguments array is missing or empty")
	}
	return arguments, nil
}

func decodeLaunchdStringArray(decoder *xml.Decoder, array xml.StartElement) ([]string, error) {
	var values []string
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch element := token.(type) {
		case xml.EndElement:
			if element.Name == array.Name {
				return values, nil
			}
		case xml.StartElement:
			if element.Name.Local != "string" {
				return nil, fmt.Errorf("ProgramArguments contains unsupported <%s> value", element.Name.Local)
			}
			var value string
			if err := decoder.DecodeElement(&value, &element); err != nil {
				return nil, err
			}
			values = append(values, value)
		}
	}
}

func readNativeServiceShellMetadata(homeDir string) (nativeServiceMetadata, bool, error) {
	path := nativeServiceMetadataPath(homeDir)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nativeServiceMetadata{}, false, nil
	}
	if err != nil {
		return nativeServiceMetadata{}, false, fmt.Errorf("read service shell metadata %q: %w", path, err)
	}
	var metadata nativeServiceMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nativeServiceMetadata{}, false, fmt.Errorf("parse service shell metadata %q: %w", path, err)
	}
	if metadata.Version != nativeServiceMetadataVersion {
		return nativeServiceMetadata{}, false, fmt.Errorf("unsupported service metadata version %d", metadata.Version)
	}
	return metadata, true, nil
}

func nativeServiceShellMetadataMatches(metadata nativeServiceMetadata, servicePath, homeDir string) bool {
	return metadata.Manager == nativeServiceManagerLaunchd &&
		metadata.Mode == nativeServiceModeUser &&
		filepath.Clean(metadata.ServicePath) == filepath.Clean(servicePath) &&
		filepath.Clean(metadata.HomeDir) == filepath.Clean(homeDir)
}

func serviceShellRepairError(err error) error {
	return fmt.Errorf("saved service shell is invalid: %w; reinstall with --shell <bash|zsh|path> or --shell=none", err)
}

func runNativeServiceShell(args serviceArgs) error {
	if runtime.GOOS != goosDarwin {
		return errors.New("service login shells are supported only on macOS")
	}
	if args.System || os.Getenv("KANDEV_RUNNING_AS_SERVICE") != "true" ||
		os.Getenv("KANDEV_SERVICE_MANAGER") != nativeServiceManagerLaunchd ||
		os.Getenv("KANDEV_SERVICE_MODE") != nativeServiceModeUser || os.Geteuid() != os.Getuid() {
		return errors.New("internal service shell action requires a macOS Kandev user service")
	}
	shellPath, err := validateServiceShellExecutable(args.Shell)
	if err != nil {
		return err
	}
	account, err := lookupServiceShellUser(strconv.Itoa(os.Geteuid()))
	if err != nil {
		return fmt.Errorf("resolve service account home: %w", err)
	}
	if !filepath.IsAbs(account.HomeDir) {
		return errors.New("service account has no absolute home directory")
	}
	if len(os.Args) == 0 || !filepath.IsAbs(os.Args[0]) {
		return errors.New("service executable path must be absolute before login initialization")
	}
	invocation, err := buildServiceShellInvocation(shellPath, account.HomeDir, os.Args[0], os.Environ())
	if err != nil {
		return err
	}
	return replaceServiceShell(invocation)
}

func buildServiceShellInvocation(shellPath, accountHome, executable string, inheritedEnv []string) (serviceShellInvocation, error) {
	if !filepath.IsAbs(accountHome) || !filepath.IsAbs(executable) {
		return serviceShellInvocation{}, errors.New("service account home and executable path must be absolute")
	}
	env := upsertEnv(inheritedEnv, "HOME", accountHome)
	env = upsertEnv(env, "SHELL", shellPath)
	assignments := []string{"HOME=" + accountHome, "SHELL=" + shellPath}
	for _, name := range serviceShellRestoreEnvironmentKeys() {
		if value, present := environmentValue(inheritedEnv, name); present {
			assignments = append(assignments, name+"="+value)
		}
	}
	argv := []string{shellPath, "-l", "-c", serviceShellCommand, "kandev-service-shell"}
	argv = append(argv, assignments...)
	argv = append(argv, executable, "--headless")
	return serviceShellInvocation{Path: shellPath, Args: argv, Env: env}, nil
}

func serviceShellRestoreEnvironmentKeys() []string {
	return []string{
		"KANDEV_RUNNING_AS_SERVICE", "KANDEV_SERVICE_MODE", "KANDEV_SERVICE_MANAGER",
		"KANDEV_INSTALL_KIND", "KANDEV_SERVICE_METADATA", "KANDEV_HOME_DIR", "KANDEV_LOG_LEVEL",
		config.InternalConfigFileEnv, config.InternalConfigHomeFileEnv, "KANDEV_SERVER_PORT",
		"KANDEV_BUNDLE_DIR", "KANDEV_VERSION",
	}
}

func environmentValue(env []string, name string) (string, bool) {
	var value string
	present := false
	for _, entry := range env {
		key, current, ok := strings.Cut(entry, "=")
		if ok && key == name {
			value, present = current, true
		}
	}
	return value, present
}
