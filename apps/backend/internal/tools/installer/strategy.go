package installer

import "context"

const installSubcommand = "install"

// CommandSpec describes a command that an install strategy needs to run.
// The runner decides how the process tree is owned and reaped.
type CommandSpec struct {
	Path string
	Args []string
	Dir  string
	Env  map[string]string
}

// CommandRunner exposes the task environment and runs installer subprocesses.
// Agentctl supplies its process manager so npm/Go descendants follow task
// teardown on every platform.
type CommandRunner interface {
	CombinedOutput(ctx context.Context, spec CommandSpec) ([]byte, error)
	// CommandEnvironment returns the task-specific overrides that the runner
	// applies on top of the current process environment.
	CommandEnvironment() (map[string]string, error)
}

// InstallResult contains information about an installed tool binary.
type InstallResult struct {
	BinaryPath string // absolute path to installed binary
}

// Strategy is the abstraction for different install methods.
type Strategy interface {
	// Install downloads/installs the tool binary. Blocks until done.
	Install(ctx context.Context) (*InstallResult, error)
	// Name returns a human-readable name for logging.
	Name() string
}
