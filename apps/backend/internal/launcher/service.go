package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

type serviceArgs struct {
	Action      string
	System      bool
	Port        int
	HomeDir     string
	NoBootStart bool
	Follow      bool
	ShowHelp    bool
}

const (
	actionInstall   = "install"
	actionUninstall = "uninstall"
	actionRestart   = "restart"
	actionConfig    = "config"
	actionLogs      = "logs"
	actionStatus    = "status"
	actionStop      = "stop"
	flagHelp        = "--help"
	goosDarwin      = "darwin"
	serviceUnitName = "kandev.service"
)

const serviceHelp = `kandev service — install kandev as an OS-managed service

Usage:
  kandev service install [--system] [--port <port>] [--home-dir <path>] [--no-boot-start]
  kandev service uninstall [--system]
  kandev service start|stop|restart|status [--system]
  kandev service logs [-f] [--system]
  kandev service config [--system]
`

func runService(argv []string) int {
	args, err := parseServiceArgs(argv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 2
	}
	if args.ShowHelp {
		fmt.Print(serviceHelp)
		return 0
	}
	if args.Action == actionConfig {
		printServiceConfig(args)
		return 0
	}
	switch runtime.GOOS {
	case "linux":
		return runLinuxService(args)
	case goosDarwin:
		return runLaunchdService(args)
	default:
		fmt.Fprintf(os.Stderr, "[kandev] service is not supported on %s\n", runtime.GOOS)
		return 1
	}
}

func parseServiceArgs(argv []string) (serviceArgs, error) {
	if len(argv) == 0 || argv[0] == flagHelp || argv[0] == "-h" {
		return serviceArgs{Action: actionInstall, ShowHelp: true}, nil
	}
	action := argv[0]
	if !validServiceAction(action) {
		return serviceArgs{}, ParseError{Message: fmt.Sprintf("unknown service action %q", action)}
	}
	out := serviceArgs{Action: action}
	for i := 1; i < len(argv); i++ {
		next, err := parseServiceFlag(argv, i, action, &out)
		if err != nil {
			return out, err
		}
		i = next
	}
	return out, nil
}

func validServiceAction(action string) bool {
	switch action {
	case actionInstall, actionUninstall, string(CommandStart), actionStop, actionRestart, actionStatus, actionLogs, actionConfig:
		return true
	default:
		return false
	}
}

func parseServiceFlag(argv []string, i int, action string, out *serviceArgs) (int, error) {
	arg := argv[i]
	switch {
	case arg == flagHelp || arg == "-h":
		out.ShowHelp = true
	case arg == "--system":
		out.System = true
	case arg == "--no-boot-start":
		out.NoBootStart = true
	case arg == "-f" || arg == "--follow":
		out.Follow = true
	case arg == "--port":
		return parseServicePort(argv, i, out)
	case strings.HasPrefix(arg, "--port="):
		return i, parseServicePortValue(strings.TrimPrefix(arg, "--port="), out)
	case arg == "--home-dir":
		return parseServiceHomeDir(argv, i, out)
	case strings.HasPrefix(arg, "--home-dir="):
		out.HomeDir = expandHome(strings.TrimPrefix(arg, "--home-dir="))
	default:
		return i, ParseError{Message: fmt.Sprintf("unknown flag %q for kandev service %s", arg, action)}
	}
	return i, nil
}

func parseServicePort(argv []string, i int, out *serviceArgs) (int, error) {
	v, err := takeValue(argv, i, "--port")
	if err != nil {
		return i, err
	}
	if err := parseServicePortValue(v, out); err != nil {
		return i, err
	}
	return i + 1, nil
}

func parseServicePortValue(value string, out *serviceArgs) error {
	port, err := parsePort(value, "--port")
	if err != nil {
		return err
	}
	out.Port = port
	return nil
}

func parseServiceHomeDir(argv []string, i int, out *serviceArgs) (int, error) {
	v, err := takeValue(argv, i, "--home-dir")
	if err != nil {
		return i, err
	}
	out.HomeDir = expandHome(v)
	return i + 1, nil
}

func runLinuxService(args serviceArgs) int {
	unitPath := linuxUnitPath(args.System)
	switch args.Action {
	case actionInstall:
		return installSystemd(args, unitPath)
	case actionUninstall:
		_ = runCommand("systemctl", append(systemctlScope(args.System), "disable", "--now", serviceUnitName)...)
		_ = os.Remove(unitPath)
		_ = runCommand("systemctl", append(systemctlScope(args.System), "daemon-reload")...)
		return 0
	case string(CommandStart), actionStop, actionRestart, actionStatus:
		if err := runCommand("systemctl", append(systemctlScope(args.System), args.Action, serviceUnitName)...); err != nil {
			return 1
		}
		return 0
	case actionLogs:
		journalArgs := []string{"-n", "200", "--no-pager"}
		if args.System {
			journalArgs = append([]string{"-u", serviceUnitName}, journalArgs...)
		} else {
			journalArgs = append([]string{"--user-unit", serviceUnitName}, journalArgs...)
		}
		if args.Follow {
			journalArgs = journalArgs[:len(journalArgs)-2]
			journalArgs = append(journalArgs, "-f")
		}
		if err := runCommand("journalctl", journalArgs...); err != nil {
			return 1
		}
		return 0
	}
	return 1
}

func installSystemd(args serviceArgs, unitPath string) int {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	homeDir := serviceHomeDir(args)
	logDir := filepath.Join(homeDir, "logs")
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	unit := renderSystemdUnit(nativeServiceUnitInput{
		Executable: self,
		HomeDir:    homeDir,
		LogDir:     logDir,
		Port:       args.Port,
		System:     args.System,
		SystemUser: serviceUser(args.System),
	})
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	if err := runCommand("systemctl", append(systemctlScope(args.System), "daemon-reload")...); err != nil {
		return 1
	}
	if args.NoBootStart {
		if err := runCommand("systemctl", append(systemctlScope(args.System), string(CommandStart), serviceUnitName)...); err != nil {
			return 1
		}
		fmt.Println("[kandev] service installed and started")
		return 0
	}
	if err := runCommand("systemctl", append(systemctlScope(args.System), "enable", "--now", serviceUnitName)...); err != nil {
		return 1
	}
	fmt.Println("[kandev] service enabled and started")
	return 0
}

func runLaunchdService(args serviceArgs) int {
	plistPath := launchdPlistPath(args.System)
	target := "gui/" + fmt.Sprint(os.Getuid()) + "/com.kdlbs.kandev"
	domain := "gui/" + fmt.Sprint(os.Getuid())
	if args.System {
		target = "system/com.kdlbs.kandev"
		domain = "system"
	}
	switch args.Action {
	case actionInstall:
		return installLaunchd(args, plistPath, target, domain)
	case "uninstall":
		_ = runCommand("launchctl", "bootout", target)
		_ = os.Remove(plistPath)
		return 0
	case "start":
		if err := runCommand("launchctl", "kickstart", target); err != nil {
			return runCommandExit("launchctl", "bootstrap", domain, plistPath)
		}
		return 0
	case "stop":
		_ = runCommand("launchctl", "bootout", target)
		return 0
	case actionRestart:
		if err := runCommand("launchctl", "kickstart", "-k", target); err != nil {
			_ = runCommand("launchctl", "bootout", target)
			return runCommandExit("launchctl", "bootstrap", domain, plistPath)
		}
		return 0
	case "status":
		return runCommandExit("launchctl", "print", target)
	case "logs":
		logDir := filepath.Join(serviceHomeDir(args), "logs")
		tailArgs := []string{"-n", "200", filepath.Join(logDir, "service.out"), filepath.Join(logDir, "service.err")}
		if args.Follow {
			tailArgs = []string{"-f", "-n", "200", filepath.Join(logDir, "service.out"), filepath.Join(logDir, "service.err")}
		}
		return runCommandExit("tail", tailArgs...)
	}
	return 1
}

func installLaunchd(args serviceArgs, plistPath, target, domain string) int {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	homeDir := serviceHomeDir(args)
	logDir := filepath.Join(homeDir, "logs")
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	plist := renderLaunchdPlist(nativeServiceUnitInput{
		Executable:  self,
		HomeDir:     homeDir,
		LogDir:      logDir,
		Port:        args.Port,
		System:      args.System,
		SystemUser:  serviceUser(args.System),
		NoBootStart: args.NoBootStart,
	})
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	_ = runCommand("launchctl", "bootout", target)
	if err := runCommand("launchctl", "bootstrap", domain, plistPath); err != nil {
		return 1
	}
	_ = runCommand("launchctl", "enable", target)
	if args.NoBootStart {
		if err := runCommand("launchctl", "kickstart", target); err != nil {
			return 1
		}
		fmt.Println("[kandev] service loaded and started")
		return 0
	}
	fmt.Println("[kandev] service loaded and started")
	return 0
}

type nativeServiceUnitInput struct {
	Executable  string
	HomeDir     string
	LogDir      string
	Port        int
	System      bool
	SystemUser  string
	NoBootStart bool
}

func renderSystemdUnit(input nativeServiceUnitInput) string {
	env := []string{
		serviceEnvLine("KANDEV_HOME_DIR", input.HomeDir),
		serviceEnvLine("KANDEV_LOG_LEVEL", "info"),
		serviceEnvLine("PATH", "/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin:%h/.local/bin:%h/.bun/bin:%h/.opencode/bin"),
	}
	if input.Port != 0 {
		env = append(env, serviceEnvLine("KANDEV_SERVER_PORT", fmt.Sprint(input.Port)))
	}
	if v := os.Getenv("KANDEV_BUNDLE_DIR"); v != "" {
		env = append(env, serviceEnvLine("KANDEV_BUNDLE_DIR", v))
	}
	if v := os.Getenv("KANDEV_VERSION"); v != "" {
		env = append(env, serviceEnvLine("KANDEV_VERSION", v))
	}
	userLine := ""
	wantedBy := "default.target"
	if input.System {
		wantedBy = "multi-user.target"
		if input.SystemUser != "" {
			userLine = "User=" + input.SystemUser + "\n"
		}
	}
	return "# managed by kandev — regenerated by `kandev service install`\n" +
		"[Unit]\nDescription=Kandev autonomous agent platform\nAfter=network-online.target\nWants=network-online.target\n\n" +
		"[Service]\nType=simple\nExecStart=" + quoteForUnit(input.Executable) + " --headless\n" +
		userLine + strings.Join(env, "\n") + "\nRestart=on-failure\nRestartSec=5s\nKillMode=mixed\nTimeoutStopSec=30s\n\n" +
		"[Install]\nWantedBy=" + wantedBy + "\n"
}

func renderLaunchdPlist(input nativeServiceUnitInput) string {
	envEntries := [][2]string{{"KANDEV_HOME_DIR", input.HomeDir}, {"KANDEV_LOG_LEVEL", "info"}, {"PATH", "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"}}
	if input.Port != 0 {
		envEntries = append(envEntries, [2]string{"KANDEV_SERVER_PORT", fmt.Sprint(input.Port)})
	}
	if v := os.Getenv("KANDEV_BUNDLE_DIR"); v != "" {
		envEntries = append(envEntries, [2]string{"KANDEV_BUNDLE_DIR", v})
	}
	if v := os.Getenv("KANDEV_VERSION"); v != "" {
		envEntries = append(envEntries, [2]string{"KANDEV_VERSION", v})
	}
	var envXML strings.Builder
	for _, entry := range envEntries {
		envXML.WriteString("      <key>" + escapeXML(entry[0]) + "</key>\n      <string>" + escapeXML(entry[1]) + "</string>\n")
	}
	userBlock := ""
	if input.System && input.SystemUser != "" {
		userBlock = "  <key>UserName</key>\n  <string>" + escapeXML(input.SystemUser) + "</string>\n"
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!-- managed by kandev — regenerated by ` + "`kandev service install`" + ` -->
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.kdlbs.kandev</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + escapeXML(input.Executable) + `</string>
    <string>--headless</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
` + envXML.String() + `  </dict>
` + userBlock + `  <key>RunAtLoad</key>
  ` + plistBool(!input.NoBootStart) + `
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>` + escapeXML(filepath.Join(input.LogDir, "service.out")) + `</string>
  <key>StandardErrorPath</key>
  <string>` + escapeXML(filepath.Join(input.LogDir, "service.err")) + `</string>
  <key>WorkingDirectory</key>
  <string>` + escapeXML(input.HomeDir) + `</string>
</dict>
</plist>
`
}

func plistBool(value bool) string {
	if value {
		return "<true/>"
	}
	return "<false/>"
}

func serviceEnvLine(key, value string) string {
	return "Environment=" + quoteForUnit(key+"="+value)
}

func quoteForUnit(value string) string {
	if !strings.ContainsAny(value, " \\\"") {
		return value
	}
	escaped := strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "\"", "\\\"")
	return `"` + escaped + `"`
}

func escapeXML(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return replacer.Replace(value)
}

func linuxUnitPath(system bool) string {
	if system {
		return "/etc/systemd/system/kandev.service"
	}
	return filepath.Join(mustHomeDir(), ".config", "systemd", "user", serviceUnitName)
}

func launchdPlistPath(system bool) string {
	if system {
		return "/Library/LaunchDaemons/com.kdlbs.kandev.plist"
	}
	return filepath.Join(mustHomeDir(), "Library", "LaunchAgents", "com.kdlbs.kandev.plist")
}

func systemctlScope(system bool) []string {
	if system {
		return nil
	}
	return []string{"--user"}
}

func serviceHomeDir(args serviceArgs) string {
	if args.HomeDir != "" {
		return args.HomeDir
	}
	if args.System {
		return "/var/lib/kandev"
	}
	return resolveHomeDir()
}

func serviceUser(system bool) string {
	if !system {
		return ""
	}
	if sudo := os.Getenv("SUDO_USER"); sudo != "" && sudo != "root" {
		return sudo
	}
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}

func printServiceConfig(args serviceArgs) {
	fmt.Println("manager:", runtime.GOOS)
	fmt.Println("mode:", map[bool]string{true: "system", false: "user"}[args.System])
	fmt.Println("home:", serviceHomeDir(args))
	if runtime.GOOS == "linux" {
		fmt.Println("unit:", linuxUnitPath(args.System))
	}
	if runtime.GOOS == goosDarwin {
		fmt.Println("plist:", launchdPlistPath(args.System))
	}
}

func expandHome(path string) string {
	if path == "~" {
		return mustHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(mustHomeDir(), strings.TrimPrefix(path, "~/"))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func mustHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func runCommandExit(name string, args ...string) int {
	if err := runCommand(name, args...); err != nil {
		return 1
	}
	return 0
}
