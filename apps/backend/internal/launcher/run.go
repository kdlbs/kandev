package launcher

import (
	"fmt"
	"os"
	"path/filepath"
)

func runInstalled(opts Options) int {
	if opts.RuntimeVersion != "" {
		fmt.Fprintln(os.Stderr, "[kandev] --runtime-version is not implemented in the native launcher yet")
		return 1
	}
	backendPort, webPort, err := resolvePorts(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 2
	}
	ports, err := pickPorts(backendPort, webPort)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	bundle, err := resolveRuntimeBundle()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	if err := os.MkdirAll(resolveDataDir(), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	_ = os.Chmod(resolveDataDir(), 0o700)

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
		WebServer:  bundle.WebServer,
		WebCWD:     filepath.Dir(bundle.WebServer),
		Ports:      ports,
		LogLevel:   logLevel,
		Opts:       opts,
	})
}
