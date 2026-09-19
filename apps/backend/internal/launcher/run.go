package launcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func runInstalled(ctx context.Context, opts Options, build BuildInfo) int {
	startupConfig, err := loadBootstrapConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	backendPort, err := resolvePorts(opts, startupConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 2
	}
	ports, err := pickPorts(backendPort, backendPortSource(opts))
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	endpoints, err := resolveBackendEndpoints(startupConfig, ports.BackendPort)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	ports.BackendURL = endpoints.accessURL
	bundle, err := resolveRuntimeBundle()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	if err := ensureDataDirForConfig(startupConfig); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}

	logLevel := resolveLogLevelForConfig(opts, startupConfig)
	releaseTag := os.Getenv("KANDEV_VERSION")
	if releaseTag == "" {
		releaseTag = "(" + bundle.Source + ")"
	}
	return launchManaged(ctx, managedAppConfig{
		Header:     "release: " + releaseTag,
		Version:    normalizedBuildVersion(build.Version),
		Mode:       "run",
		Backend:    bundle.Launcher,
		BackendCWD: filepath.Dir(bundle.Launcher),
		Ports:      ports,
		LogLevel:   logLevel,
		Opts:       opts,
		Startup:    startupConfig,
		Endpoints:  endpoints,
	})
}
