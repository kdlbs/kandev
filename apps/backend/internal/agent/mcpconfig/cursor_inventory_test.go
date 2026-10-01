package mcpconfig

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-CURSOR-PLUGIN-MCP-001.11
func TestCursorInventorySelectsEnabledExactRevision(t *testing.T) {
	cursorHome := t.TempDir()
	pluginsDir := filepath.Join(cursorHome, "plugins")
	selectedRevision := "1111111111111111111111111111111111111111"
	newerRevision := "2222222222222222222222222222222222222222"
	selectedRoot := filepath.Join(pluginsDir, "cache", "cursor-public", "atlassian", selectedRevision)
	newerRoot := filepath.Join(pluginsDir, "cache", "cursor-public", "atlassian", newerRevision)
	for root, endpoint := range map[string]string{
		selectedRoot: "https://selected.example/mcp",
		newerRoot:    "https://newer.example/mcp",
	} {
		require.NoError(t, os.MkdirAll(root, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, "mcp.json"),
			[]byte(`{"mcpServers":{"fixture":{"url":"`+endpoint+`"}}}`), 0o600))
	}
	baseTime := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(selectedRoot, baseTime, baseTime))
	require.NoError(t, os.Chtimes(newerRoot, baseTime.Add(time.Minute), baseTime.Add(time.Minute)))
	requestCount := 0
	transport := cursorInventoryRoundTripper(func(req *http.Request) (*http.Response, error) {
		requestCount++
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "https://api2.cursor.sh/aiserver.v1.DashboardService/GetEffectiveUserPlugins", req.URL.String())
		require.Equal(t, "application/json", req.Header.Get("Content-Type"))
		require.Equal(t, "1", req.Header.Get("Connect-Protocol-Version"))
		require.Equal(t, "Bearer synthetic-cursor-token", req.Header.Get("Authorization"))
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"excludeConfiguredVariables":true}`, string(body))
		response := `{"plugins":[{"plugin":{"name":"atlassian","gitRef":"` + newerRevision + `","marketplaceId":"market-1"},"isEnabled":true,"pinnedGitRef":"` + selectedRevision + `"},{"plugin":{"name":"figma","gitRef":"` + selectedRevision + `","marketplaceId":"market-1"},"isEnabled":false},{"plugin":{"name":"omitted-enabled","gitRef":"` + selectedRevision + `","marketplaceId":"market-1"}},{"plugin":{"name":"branch-pin","gitRef":"` + selectedRevision + `","marketplaceId":"market-1"},"isEnabled":true,"pinnedGitRef":"main"}],"marketplaces":[{"id":"market-1","name":"cursor-public"}]}`
		return cursorInventoryResponse(http.StatusOK, response), nil
	})
	client := NewCursorInventoryClient(func(context.Context) (string, error) {
		return "synthetic-cursor-token", nil
	}, transport)
	inventory, err := client.Load(context.Background())
	require.NoError(t, err)
	require.Len(t, inventory.Plugins, 1)
	require.Equal(t, CursorNativePlugin{Name: "atlassian", Marketplace: "cursor-public", Revision: selectedRevision}, inventory.Plugins[0])
	require.Equal(t, 1, requestCount)

	candidates, err := DiscoverCursorPluginCandidatesWithInventory(cursorHome, inventory)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, selectedRoot, candidates[0].PluginRoot)
	require.Equal(t, "https://selected.example/mcp", candidates[0].Server.URL)
}

// @covers AC-AGENTS-CURSOR-PLUGIN-MCP-001.11, AC-AGENTS-CURSOR-PLUGIN-MCP-001.13
func TestCursorInventoryRejectsRedirectAndSecretErrors(t *testing.T) {
	const token = "synthetic-cursor-token"
	const privateBody = "synthetic-private-response"
	requests := 0
	client := NewCursorInventoryClient(func(context.Context) (string, error) { return token, nil },
		cursorInventoryRoundTripper(func(req *http.Request) (*http.Response, error) {
			requests++
			require.Equal(t, token, strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "))
			response := cursorInventoryRedirect("https://attacker.example/collect")
			response.Body = io.NopCloser(strings.NewReader(privateBody))
			return response, nil
		}))
	_, err := client.Load(context.Background())
	require.Error(t, err)
	require.NotContains(t, err.Error(), token)
	require.NotContains(t, err.Error(), privateBody)
	require.Equal(t, 1, requests, "redirect responses must not issue a second request with credentials")
}

// @covers AC-AGENTS-CURSOR-PLUGIN-MCP-001.13
func TestCursorInventoryFailureDoesNotRequestWithoutCredential(t *testing.T) {
	requests := 0
	client := NewCursorInventoryClient(func(context.Context) (string, error) {
		return "", os.ErrNotExist
	}, cursorInventoryRoundTripper(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, nil
	}))
	_, err := client.Load(context.Background())
	require.Error(t, err)
	require.Zero(t, requests)
}

func TestCursorInventoryRejectsOversizeAndInvalidResponses(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantReason string
	}{
		{name: "service error body", status: http.StatusUnauthorized, body: "synthetic-provider-secret", wantReason: "service_status"},
		{name: "oversized body", status: http.StatusOK, body: strings.Repeat("x", cursorInventoryMaxBytes+1), wantReason: "response_too_large"},
		{name: "invalid JSON", status: http.StatusOK, body: `{"plugins":`, wantReason: "response_invalid"},
		{name: "missing required shape", status: http.StatusOK, body: `{"plugins":[]}`, wantReason: "response_invalid"},
		{name: "wrong required shape", status: http.StatusOK, body: `{"plugins":null,"marketplaces":[]}`, wantReason: "response_invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewCursorInventoryClient(func(context.Context) (string, error) {
				return "synthetic-cursor-token", nil
			}, cursorInventoryRoundTripper(func(*http.Request) (*http.Response, error) {
				return cursorInventoryResponse(tt.status, tt.body), nil
			}))
			_, err := client.Load(context.Background())
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantReason)
			require.NotContains(t, err.Error(), "synthetic-cursor-token")
			require.NotContains(t, err.Error(), "synthetic-provider-secret")
		})
	}
}

func TestCursorInventoryEmptyIsSuccessAndCallerCancellationIsBounded(t *testing.T) {
	client := NewCursorInventoryClient(func(context.Context) (string, error) {
		return "synthetic-cursor-token", nil
	}, cursorInventoryRoundTripper(func(*http.Request) (*http.Response, error) {
		return cursorInventoryResponse(http.StatusOK, `{"plugins":[],"marketplaces":[]}`), nil
	}))
	inventory, err := client.Load(context.Background())
	require.NoError(t, err)
	require.Empty(t, inventory.Plugins)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	requests := 0
	canceledClient := NewCursorInventoryClient(func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}, cursorInventoryRoundTripper(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, nil
	}))
	_, err = canceledClient.Load(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "canceled")
	require.Zero(t, requests)
}

func TestCursorInventoryKeepsMarketplaceResolutionScoped(t *testing.T) {
	const commit = "3333333333333333333333333333333333333333"
	body := `{"plugins":[{"plugin":{"name":"unsupported-default","gitRef":"` + commit + `","marketplaceId":"default","marketplace":{"name":"__DEFAULT__"}},"isEnabled":true},{"plugin":{"name":"atlassian","gitRef":"` + commit + `","marketplaceId":"public","marketplace":{"name":"cursor-public"}},"isEnabled":true}],"marketplaces":[{"id":"default","name":"__DEFAULT__"},{"id":"public","name":"wrong-fallback-name"}]}`
	client := NewCursorInventoryClient(func(context.Context) (string, error) {
		return "synthetic-cursor-token", nil
	}, cursorInventoryRoundTripper(func(*http.Request) (*http.Response, error) {
		return cursorInventoryResponse(http.StatusOK, body), nil
	}))
	inventory, err := client.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, []CursorNativePlugin{{Name: "atlassian", Marketplace: "cursor-public", Revision: commit}}, inventory.Plugins)
}

func TestCursorInventoryRejectsConflictingMarketplaceID(t *testing.T) {
	inventory, err := parseCursorNativeInventory([]byte(`{"plugins":[{"plugin":{"name":"atlassian","gitRef":"1111111111111111111111111111111111111111","marketplaceId":"market-1"},"isEnabled":true}],"marketplaces":[{"id":"market-1","name":"cursor-public"},{"id":"market-1","name":"other-market"},{"id":"market-1","name":"cursor-public"}]}`))
	require.NoError(t, err)
	require.Empty(t, inventory.Plugins, "an ambiguous service ID cannot select a cache root")
}

func TestCursorInventoryAmbiguousPluginAcrossMarketplacesIsSkipped(t *testing.T) {
	const commit = "4444444444444444444444444444444444444444"
	body := `{"plugins":[{"plugin":{"name":"shared","gitRef":"` + commit + `","marketplaceId":"market-one"},"isEnabled":true},{"plugin":{"name":"shared","gitRef":"` + commit + `","marketplaceId":"market-two"},"isEnabled":true}],"marketplaces":[{"id":"market-one","name":"one"},{"id":"market-two","name":"two"}]}`
	client := NewCursorInventoryClient(func(context.Context) (string, error) {
		return "synthetic-cursor-token", nil
	}, cursorInventoryRoundTripper(func(*http.Request) (*http.Response, error) {
		return cursorInventoryResponse(http.StatusOK, body), nil
	}))
	inventory, err := client.Load(context.Background())
	require.NoError(t, err)
	require.Empty(t, inventory.Plugins)
}

func TestCursorInventoryDoesNotFollowSelectedSymlinkOrUseStaleCache(t *testing.T) {
	cursorHome := t.TempDir()
	pluginsDir := filepath.Join(cursorHome, "plugins")
	const selectedRevision = "5555555555555555555555555555555555555555"
	const staleRevision = "6666666666666666666666666666666666666666"
	pluginDir := filepath.Join(pluginsDir, "cache", "cursor-public", "atlassian")
	selectedRoot := filepath.Join(pluginDir, selectedRevision)
	staleRoot := filepath.Join(pluginDir, staleRevision)
	require.NoError(t, os.MkdirAll(pluginDir, 0o700))
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "mcp.json"),
		[]byte(`{"mcpServers":{"fixture":{"url":"https://outside.example/mcp"}}}`), 0o600))
	if err := os.Symlink(outside, selectedRoot); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	require.NoError(t, os.MkdirAll(staleRoot, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(staleRoot, "mcp.json"),
		[]byte(`{"mcpServers":{"fixture":{"url":"https://stale.example/mcp"}}}`), 0o600))

	candidates, err := DiscoverCursorPluginCandidatesWithInventory(cursorHome, CursorNativeInventory{
		Plugins: []CursorNativePlugin{{Name: "atlassian", Marketplace: "cursor-public", Revision: selectedRevision}},
	})
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestCursorInventoryConcurrentCallersUseIndependentRequests(t *testing.T) {
	const callers = 8
	var requests atomic.Int32
	client := NewCursorInventoryClient(func(context.Context) (string, error) {
		return "synthetic-cursor-token", nil
	}, cursorInventoryRoundTripper(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return cursorInventoryResponse(http.StatusOK, `{"plugins":[],"marketplaces":[]}`), nil
	}))
	results := make(chan error, callers)
	for range callers {
		go func() {
			inventory, err := client.Load(context.Background())
			if err == nil && len(inventory.Plugins) != 0 {
				err = fmt.Errorf("unexpected plugins: %d", len(inventory.Plugins))
			}
			results <- err
		}()
	}
	for range callers {
		require.NoError(t, <-results)
	}
	require.EqualValues(t, callers, requests.Load())
}

type cursorInventoryRoundTripper func(*http.Request) (*http.Response, error)

func (f cursorInventoryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func cursorInventoryResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func cursorInventoryRedirect(location string) *http.Response {
	response := cursorInventoryResponse(http.StatusTemporaryRedirect, "")
	response.Header.Set("Location", location)
	return response
}
