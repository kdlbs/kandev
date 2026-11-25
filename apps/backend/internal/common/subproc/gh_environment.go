package subproc

import (
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
)

const (
	ghSessionBusAddressEnv        = "DBUS_SESSION_BUS_ADDRESS"
	ghAutolaunchSessionBusAddress = "autolaunch:"
	ghSessionBusAssignmentPrefix  = ghSessionBusAddressEnv + "="
)

// prepareGHCommand keeps managed GitHub CLI requests from launching a session bus.
func prepareGHCommand(cmd *exec.Cmd) {
	if cmd == nil || runtime.GOOS != "linux" {
		return
	}

	env := cmd.Environ()
	address := selectGHSessionBusAddress(env, ghRuntimeDirectory())
	cmd.Env = replaceEnvironmentAssignment(env, ghSessionBusAddressEnv+"="+address)
}

func ghRuntimeDirectory() string {
	return path.Join("/run/user", strconv.Itoa(os.Getuid()))
}

func selectGHSessionBusAddress(env []string, runtimeDir string) string {
	if address, ok := environmentValue(env, ghSessionBusAddressEnv); ok &&
		address != "" && address != ghAutolaunchSessionBusAddress {
		return address
	}

	busPath := path.Join(runtimeDir, "bus")
	if _, err := os.Stat(busPath); !os.IsNotExist(err) {
		return "unix:path=" + busPath
	}

	legacyPath := path.Join(runtimeDir, "dbus-session")
	contents, err := os.ReadFile(legacyPath)
	if err == nil && strings.HasPrefix(string(contents), ghSessionBusAssignmentPrefix) {
		address := strings.TrimRight(strings.TrimPrefix(string(contents), ghSessionBusAssignmentPrefix), "\r\n")
		if strings.TrimSpace(address) != "" && address != ghAutolaunchSessionBusAddress {
			return address
		}
	}

	return "unix:path=" + busPath
}
