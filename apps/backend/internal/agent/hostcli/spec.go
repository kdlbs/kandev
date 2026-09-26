// Package hostcli describes the vendor command-line tool installed on the
// Kandev host for an agent type (for example `claude` or `codex`), and reads
// its version and, where the vendor documents a listing, its model
// catalogue.
//
// The host CLI is distinct from the managed ACP bridge that Kandev launches
// for sessions. Nothing in this package changes which executable a session
// uses, and installing or updating a CLI remains the existing agent install
// flow; see docs/specs/agents/system-design/host-cli-model-discovery.md.
package hostcli

import "strings"

// ModelSource names the documented programmatic listing a host CLI offers.
type ModelSource string

const (
	// ModelSourceNone means the CLI has no documented model listing. The
	// bridge-advertised list stays authoritative and the operator may type a
	// custom model identifier.
	ModelSourceNone ModelSource = ""
	// ModelSourceCodexAppServer lists models through `codex app-server`
	// JSON-RPC (`model/list`).
	ModelSourceCodexAppServer ModelSource = "codex_app_server"
)

// Spec is compiled, trusted metadata for one vendor CLI. Requests never
// carry any of these values.
type Spec struct {
	// DisplayName is the vendor product name shown next to the version.
	DisplayName string
	// Executable is the PATH name (or absolute path for test agents) of
	// the CLI binary.
	Executable string
	// VersionArgs are appended to the executable to print its version.
	VersionArgs []string
	// ModelSource selects the documented model listing, if any.
	ModelSource ModelSource
}

// HasModelSource reports whether the CLI offers a programmatic model list.
func (s Spec) HasModelSource() bool {
	return s.ModelSource != ModelSourceNone
}

// Valid reports whether the spec names an executable; specs without one are
// ignored by discovery.
func (s Spec) Valid() bool {
	return strings.TrimSpace(s.Executable) != ""
}

// Model is one entry of a CLI-discovered catalogue.
type Model struct {
	ID          string
	Name        string
	Description string
	IsDefault   bool
	// Meta carries vendor extras that the UI may show generically, for
	// example Codex reasoning efforts.
	Meta map[string]any
}
