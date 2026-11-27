package agents

// HarnessUpdateSpec is built-in metadata for a harness-owned updater. Package
// identifies the registry metadata to compare; UpdateCommand is trusted argv
// executed directly without a caller-selected version or channel.
type HarnessUpdateSpec struct {
	Package       string
	UpdateCommand Command
}

// HarnessUpdateAgent exposes an update independently of managed npm runtimes.
// Implementations must supply trusted metadata, never API request fields.
type HarnessUpdateAgent interface {
	HarnessUpdate() HarnessUpdateSpec
}
