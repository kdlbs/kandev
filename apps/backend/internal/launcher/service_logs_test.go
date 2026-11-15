package launcher

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// @covers AC-CLI-SERVICE-LOG-HISTORY-001.1
func TestServiceLogHistoryEntrypoint(t *testing.T) {
	if runtime.GOOS != goosLinux && runtime.GOOS != goosDarwin {
		t.Skip("service logs are supported on Linux and macOS")
	}
	original := executeServiceCommand
	t.Cleanup(func() { executeServiceCommand = original })
	var command string
	var args []string
	executeServiceCommand = func(name string, values ...string) error {
		command, args = name, append([]string(nil), values...)
		return nil
	}
	if code := Run([]string{"service", "logs", "--lines", "42"}, BuildInfo{}); code != 0 {
		t.Fatalf("Run() = %d, want success", code)
	}
	if command == "" || !slices.Contains(args, "42") {
		t.Fatalf("command = %q %q, want selected history count", command, args)
	}
}

// @covers AC-CLI-SERVICE-LOG-HISTORY-001.1
// @covers AC-CLI-SERVICE-LOG-HISTORY-001.2
func TestServiceLogHistoryEntrypointValues(t *testing.T) {
	if runtime.GOOS != goosLinux && runtime.GOOS != goosDarwin {
		t.Skip("service logs are supported on Linux and macOS")
	}
	original := executeServiceCommand
	t.Cleanup(func() { executeServiceCommand = original })
	cases := []struct {
		name  string
		flags []string
		want  string
	}{
		{"default", nil, "200"},
		{"long", []string{"--lines", "42"}, "42"},
		{"equals", []string{"--lines=1000000"}, "1000000"},
		{"short", []string{"-n", "0", "-f"}, "0"},
		{"leading zeros", []string{"--lines=00017"}, "17"},
		{"last wins", []string{"-n", "9", "--lines=5"}, "5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured []string
			executeServiceCommand = func(_ string, args ...string) error {
				captured = append([]string(nil), args...)
				return nil
			}
			argv := append([]string{"service", "logs"}, tc.flags...)
			if code := Run(argv, BuildInfo{}); code != 0 {
				t.Fatalf("Run(%q) = %d, want success", argv, code)
			}
			index := slices.Index(captured, "-n")
			if index < 0 || index+1 >= len(captured) || captured[index+1] != tc.want {
				t.Fatalf("command args = %q, want -n %s", captured, tc.want)
			}
		})
	}
}

func TestServiceLogHistoryInvalidEntrypoint(t *testing.T) {
	original := executeServiceCommand
	t.Cleanup(func() { executeServiceCommand = original })
	calls := 0
	executeServiceCommand = func(string, ...string) error { calls++; return nil }
	cases := [][]string{
		{"logs", "--lines"}, {"logs", "-n"}, {"logs", "--lines="},
		{"logs", "--lines", "--follow"}, {"logs", "-n", "-1"},
		{"logs", "--lines=-1"}, {"logs", "--lines=1.5"},
		{"logs", "--lines=1000001"}, {"logs", "--lines=18446744073709551616"},
		{"logs", "--lines=2junk"}, {"logs", "--lines=+1"}, {"logs", "--lines=１"},
		{"logs", "--lines=2", "-n", "bad"},
	}
	for _, action := range []string{"install", "uninstall", "start", "stop", "restart", "status", "config", "run", "self-update"} {
		cases = append(cases, []string{action, "--lines", "1"}, []string{action, "--lines=1"}, []string{action, "-n", "1"})
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			before := calls
			if code := Run(append([]string{"service"}, args...), BuildInfo{}); code != 2 {
				t.Fatalf("Run(%q) = %d, want usage error", args, code)
			}
			if calls != before {
				t.Fatalf("invalid input invoked the command executor")
			}
		})
	}
}

// @covers AC-CLI-SERVICE-LOG-HISTORY-001.3
// @covers AC-CLI-SERVICE-LOG-HISTORY-001.4
func TestServiceLogHistoryPlatformCommands(t *testing.T) {
	original := executeServiceCommand
	t.Cleanup(func() { executeServiceCommand = original })
	home := t.TempDir()
	out := filepath.Join(home, "logs", "service.out")
	errFile := filepath.Join(home, "logs", "service.err")
	cases := []struct {
		name    string
		argv    []string
		linux   bool
		command string
		want    []string
	}{
		{"linux user default", []string{"logs"}, true, "journalctl", []string{"--user-unit", "kandev.service", "-n", "200", "--no-pager"}},
		{"linux system follow zero", []string{"logs", "--system", "--follow", "--lines=0"}, true, "journalctl", []string{"-u", "kandev.service", "-n", "0", "-f"}},
		{"linux user count", []string{"logs", "-n", "17"}, true, "journalctl", []string{"--user-unit", "kandev.service", "-n", "17", "--no-pager"}},
		{"mac custom default", []string{"logs", "--home-dir", home}, false, "tail", []string{"-n", "200", out, errFile}},
		{"mac custom system follow zero", []string{"logs", "--system", "--home-dir", home, "-f", "-n", "0"}, false, "tail", []string{"-f", "-n", "0", out, errFile}},
		{"mac custom count", []string{"logs", "--home-dir", home, "--lines", "25"}, false, "tail", []string{"-n", "25", out, errFile}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var command string
			var captured []string
			executeServiceCommand = func(name string, args ...string) error {
				command, captured = name, append([]string(nil), args...)
				return nil
			}
			args, err := parseServiceArgs(tc.argv)
			if err != nil {
				t.Fatal(err)
			}
			var code int
			if tc.linux {
				code = runLinuxService(args, BuildInfo{})
			} else {
				code = runLaunchdService(args, BuildInfo{})
			}
			if code != 0 || command != tc.command || !slices.Equal(captured, tc.want) {
				t.Fatalf("got exit %d, %s %q; want exit 0, %s %q", code, command, captured, tc.command, tc.want)
			}
		})
	}
}
