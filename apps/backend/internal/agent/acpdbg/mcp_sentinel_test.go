package acpdbg

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPSentinelRecordsOnlyProtocolMilestones(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sentinel.jsonl")
	recorder, err := NewRecorder(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = recorder.Close() })
	sentinel := NewMCPSentinel(recorder)
	t.Cleanup(sentinel.Close)

	for _, method := range []string{"initialize", "tools/list", "tools/call"} {
		body := []byte(`{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{}}`)
		request, err := http.NewRequest(http.MethodPost, sentinel.URL(), bytes.NewReader(body))
		require.NoError(t, err)
		request.Header.Set("Mcp-Session-Id", "agent-supplied-secret")
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		_ = response.Body.Close()
		require.Equal(t, http.StatusOK, response.StatusCode)
	}

	summary := sentinel.Summary()
	require.False(t, summary.HeaderObserved)
	require.True(t, summary.HeaderMissing)
	require.True(t, summary.InitializeObserved)
	require.True(t, summary.ToolsListObserved)
	require.True(t, summary.ToolCallObserved)
	require.Equal(t, 1, summary.ToolCount)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(content), "agent-supplied-secret")
	require.NotContains(t, string(content), sentinel.credential)
	require.Contains(t, string(content), "sentinel-")
}

func TestMCPSentinelRecordsAdvertisedHeaderWithoutItsValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sentinel.jsonl")
	recorder, err := NewRecorder(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = recorder.Close() })
	sentinel := NewMCPSentinel(recorder)
	t.Cleanup(sentinel.Close)
	headers := sentinel.Headers()
	require.Len(t, headers, 1)

	request, err := http.NewRequest(http.MethodPost, sentinel.URL(), bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)))
	require.NoError(t, err)
	request.Header.Set(headers[0]["name"], headers[0]["value"])
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	_ = response.Body.Close()

	summary := sentinel.Summary()
	require.True(t, summary.HeaderObserved)
	require.False(t, summary.HeaderMissing)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(content), sentinel.credential)
	require.Contains(t, string(content), `"header_observed":true`)
}
