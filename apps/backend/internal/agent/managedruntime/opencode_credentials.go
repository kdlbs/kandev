package managedruntime

import (
	"errors"
)

var ErrOpenCodeCredentialRuntime = errors.New("OpenCode credential provisioning requires a managed V2 runtime version 2.1.2 or newer")

const OpenCodeCredentialCapability = "opencode-credential-bootstrap.v1"

// OpenCodeRuntime is the resolved launch identity, independent of ACP arguments.
type OpenCodeRuntime struct {
	Family  OpenCodeFamily `json:"family"`
	Source  OpenCodeSource `json:"source"`
	Version string         `json:"version"`
}

// CredentialCommand builds only the supported standalone credential operations.
func (r OpenCodeRuntime) CredentialCommand(operation string) ([]string, error) {
	if r.Family != OpenCodeFamilyV2 || r.Source != OpenCodeSourceManaged || !supportsOpenCodeCredentials(r.Version) ||
		(operation != "import" && operation != "export") {
		return nil, ErrOpenCodeCredentialRuntime
	}
	return append(ManagedNPMCommand(OpenCodeV2Package+"@"+r.Version, false), "auth", operation, "--standalone"), nil
}

// CredentialStoreCommand queries the public CLI for its effective database path.
func (r OpenCodeRuntime) CredentialStoreCommand() ([]string, error) {
	args, err := r.CredentialCommand("export")
	if err != nil {
		return nil, err
	}
	return append(args[:len(args)-3], "debug", "paths", "db"), nil
}

func supportsOpenCodeCredentials(raw string) bool {
	version, err := ParseStableVersion(raw)
	return err == nil && version.Major() == 2 && (version.Minor() > 1 || (version.Minor() == 1 && version.Patch() >= 2))
}
