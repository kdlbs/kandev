package manifest

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-PLUGINS-SDK-DISTRIBUTION-002.3
func TestSDKRecordMatchesHostAPIVersions(t *testing.T) {
	data, err := os.ReadFile("../../../../../pluginsdk/sdk-release.json")
	require.NoError(t, err)
	var record struct {
		APIVersions []int `json:"api_versions"`
	}
	require.NoError(t, json.Unmarshal(data, &record))
	require.Equal(t, []int{LegacyAPIVersion, CurrentAPIVersion}, record.APIVersions)
}
