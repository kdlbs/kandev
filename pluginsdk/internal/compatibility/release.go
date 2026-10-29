package compatibility

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/Masterminds/semver/v3"
)

type Record struct {
	SchemaVersion int    `json:"schema_version"`
	SDKVersion    string `json:"sdk_version"`
	HostRelease   string `json:"host_release"`
	Minimum       string `json:"min_kandev_version"`
	APIVersions   []int  `json:"api_versions"`
	ChangeKind    string `json:"change_kind"`
}

const (
	metadataLimit = 1 << 20
	lockfileLimit = 32 << 20
)

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d MiB: %s", limit>>20, path)
	}
	return data, nil
}

func ReadRecord(path string, version string) (Record, error) {
	var record Record
	data, err := readBounded(path, metadataLimit)
	if err != nil {
		return record, fmt.Errorf("read SDK release record: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&record); err != nil {
		return record, fmt.Errorf("SDK release record: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return record, fmt.Errorf("SDK release record contains trailing data")
	}
	return record, record.validate(version)
}

func stableVersion(field, raw string) (*semver.Version, error) {
	version, err := semver.StrictNewVersion(raw)
	if err != nil || version.Prerelease() != "" || version.Metadata() != "" {
		return nil, fmt.Errorf("%s must be an exact published Stable version, got %q", field, raw)
	}
	return version, nil
}

func (r Record) validate(selected string) error {
	if r.SchemaVersion != 1 {
		return fmt.Errorf("unsupported SDK record schema_version %d", r.SchemaVersion)
	}
	if _, err := stableVersion("sdk_version", r.SDKVersion); err != nil {
		return err
	}
	if r.SDKVersion != selected {
		return fmt.Errorf("SDK record version %s differs from selected version %s", r.SDKVersion, selected)
	}
	host, err := stableVersion("host_release", r.HostRelease)
	if err != nil {
		return err
	}
	floor, err := stableVersion("min_kandev_version", r.Minimum)
	if err != nil {
		return err
	}
	if floor.GreaterThan(host) {
		return fmt.Errorf("SDK min_kandev_version exceeds host_release")
	}
	if !slices.Contains([]string{"initial", "patch", "additive", "breaking"}, r.ChangeKind) {
		return fmt.Errorf("invalid SDK change_kind %q", r.ChangeKind)
	}
	seen := map[int]bool{}
	for _, api := range r.APIVersions {
		if api < 1 || seen[api] {
			return fmt.Errorf("invalid SDK api_versions")
		}
		seen[api] = true
	}
	if len(seen) == 0 {
		return fmt.Errorf("SDK api_versions is empty")
	}
	return nil
}
