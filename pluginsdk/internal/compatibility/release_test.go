package compatibility

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func releasedRecord() Record {
	return Record{SchemaVersion: 1, SDKVersion: "0.1.0", HostRelease: "0.100.0", Minimum: "0.100.0", APIVersions: []int{1, 2}, ChangeKind: "initial"}
}
func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, data, 0600))
}

// @covers AC-PLUGINS-SDK-DISTRIBUTION-002.1
func TestReleasedRecordEvidence(t *testing.T) {
	tests := []struct {
		name, selected, want string
		mutate               func(*Record)
	}{
		{"published", "0.1.0", "", func(*Record) {}},
		{"wrong selected version", "0.2.0", "version", func(*Record) {}},
		{"pseudo version", "0.1.0-20261009000000-abcdef123456", "version", func(*Record) {}},
		{"pending host", "0.1.0", "host_release", func(r *Record) { r.HostRelease = "" }},
		{"pending minimum", "0.1.0", "min_kandev_version", func(r *Record) { r.Minimum = "" }},
		{"floor exceeds host", "0.1.0", "exceeds", func(r *Record) { r.Minimum = "0.101.0" }},
		{"unknown schema", "0.1.0", "schema", func(r *Record) { r.SchemaVersion = 2 }},
		{"no APIs", "0.1.0", "api_versions", func(r *Record) { r.APIVersions = nil }},
		{"invalid API", "0.1.0", "api_versions", func(r *Record) { r.APIVersions = []int{0} }},
		{"duplicate API", "0.1.0", "api_versions", func(r *Record) { r.APIVersions = []int{2, 2} }},
		{"unknown kind", "0.1.0", "change_kind", func(r *Record) { r.ChangeKind = "automatic" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := releasedRecord()
			tt.mutate(&record)
			path := filepath.Join(t.TempDir(), "sdk-release.json")
			writeJSON(t, path, record)
			got, err := ReadRecord(path, tt.selected)
			if tt.want != "" {
				require.ErrorContains(t, err, tt.want)
				return
			}
			require.NoError(t, err)
			require.Equal(t, record, got)
		})
	}
}

// @covers AC-PLUGINS-SDK-DISTRIBUTION-002.2
func TestManifestUsesEveryConsumedSDK(t *testing.T) {
	old := releasedRecord()
	newer := releasedRecord()
	newer.SDKVersion = "0.2.0"
	newer.Minimum = "0.101.0"
	newer.HostRelease = "0.101.0"
	newer.APIVersions = []int{2}
	tests := []struct {
		name, yaml, want string
		records          []Record
	}{
		{"Go only", "api_version: 1\nmin_kandev_version: 0.100.0\nid: plugin\n", "", []Record{old}},
		{"frontend only", "api_version: 2\nmin_kandev_version: 0.101.0\n", "", []Record{newer}},
		{"mixed versions", "api_version: 2\nmin_kandev_version: 0.101.0\n", "", []Record{old, newer}},
		{"maximum floor", "api_version: 2\nmin_kandev_version: 0.100.0\n", "requires", []Record{old, newer}},
		{"every API set", "api_version: 1\nmin_kandev_version: 0.101.0\n", "api_version", []Record{old, newer}},
		{"prerelease below stable", "api_version: 2\nmin_kandev_version: 0.100.0-rc.1\n", "min_kandev_version", []Record{old}},
		{"prerelease above floor", "api_version: 2\nmin_kandev_version: 0.101.0-rc.1\n", "min_kandev_version", []Record{old}},
		{"build metadata at floor", "api_version: 2\nmin_kandev_version: 0.100.0+build.1\n", "min_kandev_version", []Record{old}},
		{"prefixed stable minimum", "api_version: 2\nmin_kandev_version: v0.100.0\n", "", []Record{old}},
		{"missing minimum", "api_version: 2\n", "min_kandev_version", []Record{old}},
		{"malformed minimum", "api_version: 2\nmin_kandev_version: later\n", "min_kandev_version", []Record{old}},
		{"missing API", "min_kandev_version: 0.101.0\n", "api_version", []Record{old}},
		{"duplicate keys", "api_version: 1\napi_version: 2\n", "manifest", []Record{old}},
		{"multiple documents", "api_version: 1\n---\napi_version: 2\n", "manifest", []Record{old}},
		{"no SDK", "api_version: 2\nmin_kandev_version: 0.101.0\n", "SDK", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckManifest([]byte(tt.yaml), tt.records...)
			if tt.want != "" {
				require.ErrorContains(t, err, tt.want)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
