package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	RemoteHelperManifestName          = "remote-helpers.json"
	RemoteHelperManifestSchemaVersion = 1
	RemoteHelperVariantStandard       = "standard"
	RemoteHelperVariantFull           = "full"
	maxRemoteHelperSizeBytes          = 256 << 20
)

var (
	remoteHelperCommitPattern  = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
	remoteHelperVersionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	remoteHelperPlatformAssets = map[string]string{
		"linux/amd64":  "agentctl-linux-amd64.gz",
		"linux/arm64":  "agentctl-linux-arm64.gz",
		"darwin/amd64": "agentctl-darwin-amd64.gz",
		"darwin/arm64": "agentctl-darwin-arm64.gz",
	}
)

// RemoteHelperManifest binds a runtime bundle to the exact helpers published
// for its release. Asset names are fixed by platform and never treated as paths.
type RemoteHelperManifest struct {
	SchemaVersion int                  `json:"schema_version"`
	Version       string               `json:"version"`
	Commit        string               `json:"commit"`
	Variant       string               `json:"variant"`
	Helpers       []RemoteHelperRecord `json:"helpers"`
}

// RemoteHelperRecord describes one compressed release asset and its
// uncompressed executable bytes.
type RemoteHelperRecord struct {
	Platform  string `json:"platform"`
	Asset     string `json:"asset"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

// ReadRemoteHelperManifest returns (nil, false, nil) when a bundle has no
// manifest. A present manifest must be valid and match the running build.
func ReadRemoteHelperManifest(bundleRoot, expectedVersion, expectedCommit string) (*RemoteHelperManifest, bool, error) {
	path := filepath.Join(bundleRoot, RemoteHelperManifestName)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, fmt.Errorf("open remote helper manifest: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, true, fmt.Errorf("stat remote helper manifest: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > 64*1024 {
		return nil, true, errors.New("remote helper manifest is not a regular file within the size limit")
	}

	decoder := json.NewDecoder(io.LimitReader(file, 64*1024))
	decoder.DisallowUnknownFields()
	var manifest RemoteHelperManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, true, fmt.Errorf("decode remote helper manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, true, errors.New("remote helper manifest contains trailing JSON")
		}
		return nil, true, fmt.Errorf("decode remote helper manifest trailer: %w", err)
	}
	if err := validateRemoteHelperManifest(manifest, expectedVersion, expectedCommit); err != nil {
		return nil, true, err
	}
	return &manifest, true, nil
}

func validateRemoteHelperManifest(manifest RemoteHelperManifest, expectedVersion, expectedCommit string) error {
	if err := validateRemoteHelperManifestIdentity(manifest, expectedVersion, expectedCommit); err != nil {
		return err
	}
	if manifest.Variant != RemoteHelperVariantStandard && manifest.Variant != RemoteHelperVariantFull {
		return fmt.Errorf("remote helper manifest variant %q is invalid", manifest.Variant)
	}
	if len(manifest.Helpers) != len(remoteHelperPlatformAssets) {
		return fmt.Errorf("remote helper manifest must contain exactly %d platform records", len(remoteHelperPlatformAssets))
	}
	seen := make(map[string]struct{}, len(manifest.Helpers))
	for _, helper := range manifest.Helpers {
		if _, ok := seen[helper.Platform]; ok {
			return fmt.Errorf("remote helper manifest contains duplicate platform %q", helper.Platform)
		}
		if err := validateRemoteHelperRecord(helper); err != nil {
			return err
		}
		seen[helper.Platform] = struct{}{}
	}
	return nil
}

func validateRemoteHelperManifestIdentity(manifest RemoteHelperManifest, expectedVersion, expectedCommit string) error {
	if manifest.SchemaVersion != RemoteHelperManifestSchemaVersion {
		return fmt.Errorf("unsupported remote helper manifest schema version %d", manifest.SchemaVersion)
	}
	if !remoteHelperVersionPattern.MatchString(manifest.Version) {
		return fmt.Errorf("remote helper manifest version %q is invalid", manifest.Version)
	}
	if !remoteHelperCommitPattern.MatchString(manifest.Commit) {
		return fmt.Errorf("remote helper manifest commit must be a full lowercase Git SHA")
	}
	if expectedVersion == "" || expectedCommit == "" || expectedCommit == "unknown" ||
		manifest.Version != expectedVersion || manifest.Commit != expectedCommit {
		return fmt.Errorf("remote helper manifest identity does not match the running build")
	}
	return nil
}

func validateRemoteHelperRecord(helper RemoteHelperRecord) error {
	asset, supported := remoteHelperPlatformAssets[helper.Platform]
	if !supported {
		return fmt.Errorf("remote helper manifest platform %q is unsupported", helper.Platform)
	}
	if helper.Asset != asset || filepath.Base(helper.Asset) != helper.Asset || strings.Contains(helper.Asset, "..") {
		return fmt.Errorf("remote helper manifest asset for %s is invalid", helper.Platform)
	}
	if len(helper.SHA256) != sha256.Size*2 {
		return fmt.Errorf("remote helper manifest digest for %s is invalid", helper.Platform)
	}
	if _, err := hex.DecodeString(helper.SHA256); err != nil || strings.ToLower(helper.SHA256) != helper.SHA256 {
		return fmt.Errorf("remote helper manifest digest for %s is invalid", helper.Platform)
	}
	if helper.SizeBytes <= 0 || helper.SizeBytes > maxRemoteHelperSizeBytes {
		return fmt.Errorf("remote helper manifest size for %s is invalid", helper.Platform)
	}
	return nil
}

func remoteHelperForPlatform(manifest *RemoteHelperManifest, platform SSHRemotePlatform) (RemoteHelperRecord, bool) {
	if manifest == nil {
		return RemoteHelperRecord{}, false
	}
	want := platform.GOOS + "/" + platform.GOARCH
	for _, record := range manifest.Helpers {
		if record.Platform == want {
			return record, true
		}
	}
	return RemoteHelperRecord{}, false
}

// RemoteHelperRecordFor returns the manifest entry corresponding to a bundled
// helper executable name such as agentctl-linux-amd64.
func RemoteHelperRecordFor(manifest *RemoteHelperManifest, helperName string) (RemoteHelperRecord, bool) {
	if manifest == nil {
		return RemoteHelperRecord{}, false
	}
	for platform, asset := range remoteHelperPlatformAssets {
		if strings.TrimSuffix(asset, ".gz") == helperName {
			for _, record := range manifest.Helpers {
				if record.Platform == platform {
					return record, true
				}
			}
		}
	}
	return RemoteHelperRecord{}, false
}

func validateBundledRemoteHelper(path string, record RemoteHelperRecord) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open bundled helper %s: %w", record.Platform, err)
	}
	defer func() { _ = file.Close() }()
	hasher := sha256.New()
	size, err := io.Copy(hasher, io.LimitReader(file, maxRemoteHelperSizeBytes+1))
	if err != nil {
		return fmt.Errorf("read bundled helper %s: %w", record.Platform, err)
	}
	if size != record.SizeBytes || hex.EncodeToString(hasher.Sum(nil)) != record.SHA256 {
		return fmt.Errorf("bundled helper %s does not match remote helper manifest", record.Platform)
	}
	return nil
}

// ValidateBundledRemoteHelper checks the uncompressed executable in a full
// runtime bundle against the selected manifest record.
func ValidateBundledRemoteHelper(path string, record RemoteHelperRecord) error {
	return validateBundledRemoteHelper(path, record)
}
