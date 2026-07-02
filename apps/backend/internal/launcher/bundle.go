package launcher

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

type runtimeBundle struct {
	Dir      string
	Launcher string
	Source   string
}

type agentctlRemoteHelper struct {
	Name  string
	Label string
	// MustBeSigned requires the Mach-O helper to carry an ad-hoc (or real) code
	// signature. Apple Silicon (AMFI) SIGKILLs unsigned darwin/arm64 binaries on
	// launch, so an unsigned arm64 helper would fail on every Mac it's uploaded
	// to — catch that at bundle-validation time, not at remote-launch time.
	// darwin/amd64 is intentionally exempt: Intel macOS does not enforce code
	// signing at exec, and Go's linker does not ad-hoc-sign amd64 cross-builds.
	MustBeSigned bool
}

var requiredAgentctlRemoteHelpers = []agentctlRemoteHelper{
	{Name: "agentctl-linux-amd64", Label: "agentctl linux/amd64 helper"},
	{Name: "agentctl-linux-arm64", Label: "agentctl linux/arm64 helper"},
	{Name: "agentctl-darwin-arm64", Label: "agentctl darwin/arm64 helper", MustBeSigned: true},
	{Name: "agentctl-darwin-amd64", Label: "agentctl darwin/amd64 helper"},
}

func resolveRuntimeBundle() (runtimeBundle, error) {
	dir := os.Getenv("KANDEV_BUNDLE_DIR")
	if dir == "" {
		return runtimeBundle{}, fmt.Errorf("no Kandev runtime found; KANDEV_BUNDLE_DIR is not set")
	}
	return validateRuntimeBundle(dir, "env")
}

func validateRuntimeBundle(dir, source string) (runtimeBundle, error) {
	launcher := filepath.Join(dir, "bin", executableName("kandev"))
	if !exists(launcher) {
		return runtimeBundle{}, fmt.Errorf("launcher binary not found in bundle at %s", launcher)
	}
	agentctl := filepath.Join(dir, "bin", executableName("agentctl"))
	if !exists(agentctl) {
		return runtimeBundle{}, fmt.Errorf("agentctl binary not found in bundle at %s", agentctl)
	}
	for _, helper := range requiredAgentctlRemoteHelpers {
		path := filepath.Join(dir, "bin", helper.Name)
		if !exists(path) {
			return runtimeBundle{}, fmt.Errorf("%s not found in bundle at %s", helper.Label, path)
		}
		if helper.MustBeSigned {
			signed, ok := machoHasCodeSignature(path)
			if !ok {
				// Fail closed: a helper we can't parse as a thin arm64 Mach-O
				// (read error, truncation, unexpected/fat layout) can't be
				// verified as signed, and an unverifiable darwin/arm64 helper
				// would still be SIGKILLed by Apple Silicon at launch.
				return runtimeBundle{}, fmt.Errorf(
					"%s at %s is not a parsable thin darwin/arm64 Mach-O; cannot verify its "+
						"code signature (rebuild it via 'make -C apps/backend build-agentctl-remote')",
					helper.Label, path)
			}
			if !signed {
				return runtimeBundle{}, fmt.Errorf(
					"%s at %s is not code-signed; Apple Silicon will refuse to run it "+
						"(build it via 'make -C apps/backend build-agentctl-remote', which ad-hoc-signs darwin helpers)",
					helper.Label, path)
			}
		}
	}
	return runtimeBundle{Dir: dir, Launcher: launcher, Source: source}, nil
}

// machoHasCodeSignature reports whether a thin 64-bit Mach-O carries an
// LC_CODE_SIGNATURE load command. The second return is false when the file
// can't be parsed as a thin Mach-O (e.g. unexpected format) so callers can
// treat "undeterminable" as non-fatal rather than failing on a parse quirk.
func machoHasCodeSignature(path string) (signed bool, ok bool) {
	const (
		machoMagic64LE     = 0xfeedfacf // MH_MAGIC_64, written little-endian on disk
		lcCodeSignature    = 0x1d       // LC_CODE_SIGNATURE
		machoHeaderSize64  = 32
		loadCommandHdrSize = 8
	)
	data, err := os.ReadFile(path)
	if err != nil || len(data) < machoHeaderSize64 {
		return false, false
	}
	if binary.LittleEndian.Uint32(data[0:4]) != machoMagic64LE {
		return false, false // not a thin LE 64-bit Mach-O (fat/other) — don't judge
	}
	ncmds := binary.LittleEndian.Uint32(data[16:20])
	off := machoHeaderSize64
	for i := uint32(0); i < ncmds; i++ {
		if off+loadCommandHdrSize > len(data) {
			return false, false // truncated load-command table
		}
		cmd := binary.LittleEndian.Uint32(data[off : off+4])
		cmdSize := binary.LittleEndian.Uint32(data[off+4 : off+8])
		if cmd == lcCodeSignature {
			return true, true
		}
		if cmdSize < loadCommandHdrSize {
			return false, false // malformed
		}
		off += int(cmdSize)
	}
	return false, true
}

func executableName(name string) string {
	if os.PathSeparator == '\\' {
		return name + ".exe"
	}
	return name
}
