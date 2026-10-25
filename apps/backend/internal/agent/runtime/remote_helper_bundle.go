package runtime

import "github.com/kandev/kandev/internal/agent/runtime/lifecycle"

const (
	RemoteHelperManifestName    = lifecycle.RemoteHelperManifestName
	RemoteHelperVariantStandard = lifecycle.RemoteHelperVariantStandard
)

type RemoteHelperManifest = lifecycle.RemoteHelperManifest
type RemoteHelperRecord = lifecycle.RemoteHelperRecord

// ReadRemoteHelperManifest validates bundle metadata at the runtime boundary.
func ReadRemoteHelperManifest(bundleRoot, expectedVersion, expectedCommit string) (*RemoteHelperManifest, bool, error) {
	return lifecycle.ReadRemoteHelperManifest(bundleRoot, expectedVersion, expectedCommit)
}

// RemoteHelperRecordFor returns the manifest entry matching a packaged helper.
func RemoteHelperRecordFor(manifest *RemoteHelperManifest, helperName string) (RemoteHelperRecord, bool) {
	return lifecycle.RemoteHelperRecordFor(manifest, helperName)
}

// ValidateBundledRemoteHelper checks a full bundle's helper against its manifest.
func ValidateBundledRemoteHelper(path string, record RemoteHelperRecord) error {
	return lifecycle.ValidateBundledRemoteHelper(path, record)
}
