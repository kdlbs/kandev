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
	goosLinux       = "linux"
	goosDarwin      = "darwin"
	managedMarker   = "managed by kandev"
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

var (
	executeServiceCommand = defaultRunCommand
	servicePrintln        = func(message string) { fmt.Println(message) }
)

func runService(argv []string, build BuildInfo) int {
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
	case goosLinux:
		return runLinuxService(args, build)
	case goosDarwin:
		return runLaunchdService(args, build)
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

func runLinuxService(args serviceArgs, build BuildInfo) int {
	unitPath := linuxUnitPath(args.System)
	switch args.Action {
	case actionInstall:
		return installSystemd(args, build, unitPath)
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
		if err := runCommand("journalctl", buildJournalArgs(args)...); err != nil {
			return 1
		}
		return 0
	}
	return 1
}

func buildJournalArgs(args serviceArgs) []string {
	journalArgs := []string{"-n", "200", "--no-pager"}
	if args.System {
		journalArgs = append([]string{"-u", serviceUnitName}, journalArgs...)
	} else {
		journalArgs = append([]string{"--user-unit", serviceUnitName}, journalArgs...)
	}
	if args.Follow {
		journalArgs = journalArgs[:len(journalArgs)-1]
		journalArgs = append(journalArgs, "-f")
	}
	return journalArgs
}

func installSystemd(args serviceArgs, build BuildInfo, unitPath string) int {
	self, err := executablePath()
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
		Executable:   self,
		HomeDir:      homeDir,
		LogDir:       logDir,
		Port:         args.Port,
		System:       args.System,
		SystemUser:   serviceUser(args.System),
		BundleDir:    serviceBundleDir(self),
		Version:      serviceVersion(build.Version),
		PathPrefixes: serviceNodeToolBinDirs(),
	})
	if err := backupUnmanagedServiceFile(unitPath); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
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
		servicePrintln("[kandev] service installed and started (boot-start disabled)")
		return 0
	}
	if err := runCommand("systemctl", append(systemctlScope(args.System), "enable", "--now", serviceUnitName)...); err != nil {
		return 1
	}
	servicePrintln("[kandev] service enabled and started")
	return 0
}

func runLaunchdService(args serviceArgs, build BuildInfo) int {
	plistPath := launchdPlistPath(args.System)
	target := "gui/" + fmt.Sprint(os.Getuid()) + "/com.kdlbs.kandev"
	domain := "gui/" + fmt.Sprint(os.Getuid())
	if args.System {
		target = "system/com.kdlbs.kandev"
		domain = "system"
	}
	switch args.Action {
	case actionInstall:
		return installLaunchd(args, build, plistPath, target, domain)
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

func installLaunchd(args serviceArgs, build BuildInfo, plistPath, target, domain string) int {
	self, err := executablePath()
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
		Executable:   self,
		HomeDir:      homeDir,
		LogDir:       logDir,
		Port:         args.Port,
		System:       args.System,
		SystemUser:   serviceUser(args.System),
		NoBootStart:  args.NoBootStart,
		BundleDir:    serviceBundleDir(self),
		Version:      serviceVersion(build.Version),
		PathPrefixes: serviceNodeToolBinDirs(),
	})
	if err := backupUnmanagedServiceFile(plistPath); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
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
		servicePrintln("[kandev] service loaded and started (boot-start disabled)")
		return 0
	}
	servicePrintln("[kandev] service loaded, enabled, and started")
	return 0
}

type nativeServiceUnitInput struct {
	Executable   string
	HomeDir      string
	LogDir       string
	Port         int
	System       bool
	SystemUser   string
	NoBootStart  bool
	BundleDir    string
	Version      string
	PathPrefixes []string
}

const (
	systemdServicePath = "/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin:%h/.local/bin:%h/.bun/bin:%h/.opencode/bin"
	launchdServicePath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
)

func serviceNodeToolBinDirs() []string {
	var dirs []string
	addPath := func(value string) {
		if value == "" {
			return
		}
		dirs = append(dirs, filepath.Dir(value))
	}
	addPath(os.Getenv("npm_node_execpath"))
	for _, name := range []string{"node", "npm", "npx"} {
		if resolved, err := exec.LookPath(name); err == nil {
			addPath(resolved)
		}
	}
	return cleanServicePathDirs(dirs)
}

func servicePathWithPrefixes(base string, prefixes []string) string {
	baseParts := strings.Split(base, ":")
	seen := make(map[string]struct{}, len(baseParts)+len(prefixes))
	for _, part := range baseParts {
		seen[part] = struct{}{}
	}
	out := make([]string, 0, len(prefixes)+len(baseParts))
	for _, dir := range cleanServicePathDirs(prefixes) {
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	out = append(out, baseParts...)
	return strings.Join(out, ":")
}

func cleanServicePathDirs(dirs []string) []string {
	seen := make(map[string]struct{}, len(dirs))
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if dir == "" || strings.ContainsAny(dir, ":\n\r") || strings.ContainsRune(dir, 0) {
			continue
		}
		clean := filepath.Clean(dir)
		if !filepath.IsAbs(clean) {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func renderSystemdUnit(input nativeServiceUnitInput) string {
	pathValue := servicePathWithPrefixes(systemdServicePath, input.PathPrefixes)
	env := []string{
		serviceEnvLine("KANDEV_HOME_DIR", input.HomeDir),
		serviceEnvLine("KANDEV_LOG_LEVEL", "info"),
		serviceEnvLineAllowSpecifiers("PATH", pathValue),
	}
	if input.Port != 0 {
		env = append(env, serviceEnvLine("KANDEV_SERVER_PORT", fmt.Sprint(input.Port)))
	}
	if input.BundleDir != "" {
		env = append(env, serviceEnvLine("KANDEV_BUNDLE_DIR", input.BundleDir))
	}
	if input.Version != "" {
		env = append(env, serviceEnvLine("KANDEV_VERSION", input.Version))
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
	pathValue := servicePathWithPrefixes(launchdServicePath, input.PathPrefixes)
	envEntries := [][2]string{{"KANDEV_HOME_DIR", input.HomeDir}, {"KANDEV_LOG_LEVEL", "info"}, {"PATH", pathValue}}
	if input.Port != 0 {
		envEntries = append(envEntries, [2]string{"KANDEV_SERVER_PORT", fmt.Sprint(input.Port)})
	}
	if input.BundleDir != "" {
		envEntries = append(envEntries, [2]string{"KANDEV_BUNDLE_DIR", input.BundleDir})
	}
	if input.Version != "" {
		envEntries = append(envEntries, [2]string{"KANDEV_VERSION", input.Version})
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

func serviceEnvLineAllowSpecifiers(key, value string) string {
	return "Environment=" + quoteForUnitAllowSpecifiers(key+"="+value)
}

func quoteForUnit(value string) string {
	return quoteForUnitValue(value, true)
}

func quoteForUnitAllowSpecifiers(value string) string {
	return quoteForUnitValue(value, false)
}

func quoteForUnitValue(value string, escapeSystemdSpecifiers bool) string {
	chars := " \\\"$"
	if escapeSystemdSpecifiers {
		chars += "%"
	}
	if !strings.ContainsAny(value, chars) {
		return value
	}
	replacements := []string{"\\", "\\\\", "$", "$$", "\"", "\\\""}
	if escapeSystemdSpecifiers {
		replacements = append(replacements, "%", "%%")
	}
	escaped := strings.NewReplacer(replacements...).Replace(value)
	return `"` + escaped + `"`
}

func serviceBundleDir(executable string) string {
	if v := os.Getenv("KANDEV_BUNDLE_DIR"); v != "" {
		return v
	}
	return filepath.Dir(filepath.Dir(executable))
}

func serviceVersion(buildVersion string) string {
	if v := os.Getenv("KANDEV_VERSION"); v != "" {
		return v
	}
	return buildVersion
}

func backupUnmanagedServiceFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), managedMarker) {
		return nil
	}
	backupPath := path + ".bak"
	if err := os.WriteFile(backupPath, data, info.Mode().Perm()); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[kandev] WARNING: existing unmanaged service file backed up to %s\n", backupPath)
	return nil
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
	if runtime.GOOS == goosLinux {
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
	return executeServiceCommand(name, args...)
}

func defaultRunCommand(name string, args ...string) error {
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
