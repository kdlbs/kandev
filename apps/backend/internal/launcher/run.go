package launcher

import (
	"fmt"
	"os"
	"path/filepath"
)

func runInstalled(opts Options) int {
	backendPort, err := resolvePorts(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 2
	}
	ports, err := pickPorts(backendPort)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	bundle, err := resolveRuntimeBundle()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	if err := ensureDataDir(); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}

	logLevel := resolveLogLevel(opts)
	releaseTag := os.Getenv("KANDEV_VERSION")
	if releaseTag == "" {
		releaseTag = "(" + bundle.Source + ")"
	}
	return runManagedApp(managedAppConfig{
		Header:     "release: " + releaseTag,
		Mode:       "run",
		Backend:    bundle.Launcher,
		BackendCWD: filepath.Dir(bundle.Launcher),
		Ports:      ports,
		LogLevel:   logLevel,
		Opts:       opts,
	})
}
