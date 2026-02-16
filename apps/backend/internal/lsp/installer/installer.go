package installer

import "context"

// InstallResult contains information about an installed language server binary.
type InstallResult struct {
	BinaryPath string // absolute path to installed binary
}

// Strategy is the abstraction for different install methods.
type Strategy interface {
	// Install downloads/installs the language server. Blocks until done.
	Install(ctx context.Context) (*InstallResult, error)
	// Name returns a human-readable name for logging.
	Name() string
}
