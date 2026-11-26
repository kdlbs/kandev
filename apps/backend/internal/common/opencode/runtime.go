package opencode

import (
	"errors"
	"regexp"

	"github.com/Masterminds/semver/v3"
	"github.com/kandev/kandev/internal/common/managednpm"
)

const (
	PackageV1              = "opencode-ai"
	PackageV2              = "@opencode/cli"
	CredentialCapability   = "opencode-credential-bootstrap.v1"
	CredentialStoreEmpty   = "empty"
	CredentialStorePresent = "present"
	CredentialStorePending = "pending"
)

var (
	ErrCredentialRuntime = errors.New("OpenCode credential provisioning requires a managed V2 runtime version 2.1.2 or newer")
	stableVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)
)

type Family string

const (
	FamilyV1 Family = "v1"
	FamilyV2 Family = "v2"
)

type Source string

const (
	SourceManaged Source = "managed"
	SourceNative  Source = "native"
)

// Runtime is the resolved OpenCode runtime identity shared by the backend
// lifecycle and the standalone agentctl process.
type Runtime struct {
	Family  Family `json:"family"`
	Source  Source `json:"source"`
	Version string `json:"version"`
}

// CredentialCommand builds a supported standalone credential operation.
func (r Runtime) CredentialCommand(operation string) ([]string, error) {
	if r.Family != FamilyV2 || r.Source != SourceManaged || !supportsCredentials(r.Version) ||
		(operation != "import" && operation != "export") {
		return nil, ErrCredentialRuntime
	}
	args := []string{"npx", "--yes", "--prefer-offline"}
	args = append(args, managednpm.NPMProjectPrefixArgs()...)
	args = append(args, PackageV2+"@"+r.Version, "auth", operation, "--standalone")
	return args, nil
}

// CredentialStoreCommand asks the public CLI for its effective database path.
func (r Runtime) CredentialStoreCommand() ([]string, error) {
	args, err := r.CredentialCommand("export")
	if err != nil {
		return nil, err
	}
	return append(args[:len(args)-3], "debug", "paths", "db"), nil
}

func supportsCredentials(raw string) bool {
	if !stableVersionPattern.MatchString(raw) {
		return false
	}
	version, err := semver.StrictNewVersion(raw)
	return err == nil && version.Major() == 2 &&
		(version.Minor() > 1 || (version.Minor() == 1 && version.Patch() >= 2))
}
