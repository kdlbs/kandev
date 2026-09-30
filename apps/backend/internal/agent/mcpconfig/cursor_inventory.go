package mcpconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	cursorInventoryEndpoint = "https://api2.cursor.sh/aiserver.v1.DashboardService/GetEffectiveUserPlugins"
	cursorInventoryTimeout  = 5 * time.Second
	cursorInventoryMaxBytes = 2 * 1024 * 1024
)

var (
	cursorInventoryRevisionPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	cursorInventorySegmentPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// CursorNativePlugin identifies the immutable cache root selected by Cursor.
type CursorNativePlugin struct {
	Name        string
	Marketplace string
	Revision    string
}

// CursorNativeInventory contains the enabled plugins from Cursor's account service.
type CursorNativeInventory struct {
	Plugins []CursorNativePlugin
}

// CursorInventoryCredentialReader reads Cursor's existing account access token.
type CursorInventoryCredentialReader func(context.Context) (string, error)

// CursorInventoryClient reads the current account's enabled plugin inventory.
type CursorInventoryClient struct {
	readToken CursorInventoryCredentialReader
	transport http.RoundTripper
}

// CursorInventoryError contains only a stable, sanitized failure reason.
type CursorInventoryError struct {
	Reason string
}

func (e *CursorInventoryError) Error() string {
	return "Cursor plugin inventory unavailable: " + e.Reason
}

// NewCursorInventoryClient creates an inventory client with injected native boundaries.
func NewCursorInventoryClient(reader CursorInventoryCredentialReader, transport http.RoundTripper) *CursorInventoryClient {
	return &CursorInventoryClient{readToken: reader, transport: transport}
}

// Load returns the current enabled plugin inventory.
func (c *CursorInventoryClient) Load(ctx context.Context) (CursorNativeInventory, error) {
	if c == nil || c.readToken == nil {
		return CursorNativeInventory{}, cursorInventoryFailure("credential_reader_unavailable")
	}
	requestCtx, cancel := context.WithTimeout(ctx, cursorInventoryTimeout)
	defer cancel()
	token, err := c.readToken(requestCtx)
	if err != nil {
		return CursorNativeInventory{}, cursorInventoryFailure(inventoryContextReason(requestCtx, "credentials_unavailable"))
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 8192 {
		return CursorNativeInventory{}, cursorInventoryFailure("credentials_unavailable")
	}

	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, cursorInventoryEndpoint,
		bytes.NewBufferString(`{"excludeConfiguredVariables":true}`))
	if err != nil {
		return CursorNativeInventory{}, cursorInventoryFailure("request_invalid")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("Authorization", "Bearer "+token)

	transport := c.transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return CursorNativeInventory{}, cursorInventoryFailure(inventoryContextReason(requestCtx, "request_failed"))
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return CursorNativeInventory{}, cursorInventoryFailure("service_status")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, cursorInventoryMaxBytes+1))
	if err != nil {
		return CursorNativeInventory{}, cursorInventoryFailure("response_unreadable")
	}
	if len(data) > cursorInventoryMaxBytes {
		return CursorNativeInventory{}, cursorInventoryFailure("response_too_large")
	}
	inventory, err := parseCursorNativeInventory(data)
	if err != nil {
		return CursorNativeInventory{}, cursorInventoryFailure("response_invalid")
	}
	return inventory, nil
}

// LoadCursorNativeInventory uses the verified host credential store and service.
func LoadCursorNativeInventory(ctx context.Context) (CursorNativeInventory, error) {
	return NewCursorInventoryClient(readNativeCursorAccessToken, nil).Load(ctx)
}

func cursorInventoryFailure(reason string) error {
	return &CursorInventoryError{Reason: reason}
}

func inventoryContextReason(ctx context.Context, fallback string) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "canceled"
	}
	return fallback
}

func parseCursorNativeInventory(data []byte) (CursorNativeInventory, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return CursorNativeInventory{}, errors.New("invalid inventory object")
	}
	var pluginRows []json.RawMessage
	var marketplaceRows []json.RawMessage
	if err := json.Unmarshal(root["plugins"], &pluginRows); err != nil || pluginRows == nil {
		return CursorNativeInventory{}, errors.New("invalid plugin list")
	}
	if err := json.Unmarshal(root["marketplaces"], &marketplaceRows); err != nil || marketplaceRows == nil {
		return CursorNativeInventory{}, errors.New("invalid marketplace list")
	}
	marketplaces, err := parseCursorMarketplaces(marketplaceRows)
	if err != nil {
		return CursorNativeInventory{}, err
	}
	var selected []CursorNativePlugin
	for _, row := range pluginRows {
		plugin, ok := parseEnabledCursorPlugin(row, marketplaces)
		if ok {
			selected = append(selected, plugin)
		}
	}
	return CursorNativeInventory{Plugins: uniqueCursorNativePlugins(selected)}, nil
}

func parseCursorMarketplaces(rows []json.RawMessage) (map[string]string, error) {
	marketplaces := make(map[string]string, len(rows))
	ambiguous := make(map[string]struct{})
	for _, row := range rows {
		var entry struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(row, &entry); err != nil || entry.ID == "" || entry.Name == "" {
			continue
		}
		if _, conflicted := ambiguous[entry.ID]; conflicted {
			continue
		}
		if previous, exists := marketplaces[entry.ID]; exists {
			if previous != entry.Name {
				delete(marketplaces, entry.ID)
				ambiguous[entry.ID] = struct{}{}
			}
			continue
		}
		marketplaces[entry.ID] = entry.Name
	}
	return marketplaces, nil
}

func parseEnabledCursorPlugin(row json.RawMessage, marketplaces map[string]string) (CursorNativePlugin, bool) {
	var effective struct {
		Plugin       json.RawMessage `json:"plugin"`
		IsEnabled    *bool           `json:"isEnabled"`
		PinnedGitRef *string         `json:"pinnedGitRef"`
	}
	if json.Unmarshal(row, &effective) != nil || effective.IsEnabled == nil || !*effective.IsEnabled {
		return CursorNativePlugin{}, false
	}
	var plugin struct {
		Name          string          `json:"name"`
		GitRef        string          `json:"gitRef"`
		MarketplaceID string          `json:"marketplaceId"`
		Marketplace   json.RawMessage `json:"marketplace"`
	}
	if json.Unmarshal(effective.Plugin, &plugin) != nil || !isSafeCursorInventorySegment(plugin.Name) {
		return CursorNativePlugin{}, false
	}
	marketplace, ok := resolveCursorMarketplace(plugin.MarketplaceID, plugin.Marketplace, marketplaces)
	if !ok || !isSafeCursorInventorySegment(marketplace) {
		return CursorNativePlugin{}, false
	}
	revision := plugin.GitRef
	if effective.PinnedGitRef != nil && strings.TrimSpace(*effective.PinnedGitRef) != "" {
		revision = *effective.PinnedGitRef
	}
	if !cursorInventoryRevisionPattern.MatchString(revision) {
		return CursorNativePlugin{}, false
	}
	return CursorNativePlugin{Name: plugin.Name, Marketplace: marketplace, Revision: strings.ToLower(revision)}, true
}

func resolveCursorMarketplace(marketplaceID string, embedded json.RawMessage, marketplaces map[string]string) (string, bool) {
	if len(embedded) > 0 && string(embedded) != "null" {
		var identity struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(embedded, &identity); err != nil {
			return "", false
		}
		if identity.Name != "" {
			return identity.Name, true
		}
		if identity.ID != "" {
			name, exists := marketplaces[identity.ID]
			return name, exists
		}
		return "", false
	}
	name, exists := marketplaces[marketplaceID]
	return name, exists
}

func uniqueCursorNativePlugins(plugins []CursorNativePlugin) []CursorNativePlugin {
	byIdentity := make(map[string]CursorNativePlugin, len(plugins))
	duplicates := make(map[string]struct{})
	nameCounts := make(map[string]int)
	for _, plugin := range plugins {
		identity := strings.ToLower(plugin.Marketplace + "/" + plugin.Name)
		if previous, exists := byIdentity[identity]; exists {
			if previous.Revision != plugin.Revision {
				duplicates[identity] = struct{}{}
			}
			continue
		}
		byIdentity[identity] = plugin
		nameCounts[strings.ToLower(plugin.Name)]++
	}
	result := make([]CursorNativePlugin, 0, len(byIdentity))
	for identity, plugin := range byIdentity {
		if _, ambiguous := duplicates[identity]; ambiguous || nameCounts[strings.ToLower(plugin.Name)] > 1 {
			continue
		}
		result = append(result, plugin)
	}
	return result
}

func isSafeCursorInventorySegment(value string) bool {
	return value != "." && value != ".." && cursorInventorySegmentPattern.MatchString(value)
}
