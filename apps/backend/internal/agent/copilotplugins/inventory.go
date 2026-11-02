package copilotplugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/managednpm"
	"github.com/tailscale/hujson"
)

// Status is the closed top-level discovery state. A failed command is never a
// successful empty inventory.
type Status string

const (
	StatusReady       Status = "ready"
	StatusUnavailable Status = "unavailable"
	StatusUnsupported Status = "unsupported"
)

// ReasonCode is a bounded, user-safe explanation for an unavailable or
// unsupported snapshot or row.
type ReasonCode string

const (
	ReasonCLIUnavailable       ReasonCode = "cli_unavailable"
	ReasonTimeout              ReasonCode = "timeout"
	ReasonCancelled            ReasonCode = "cancelled"
	ReasonOutputLimit          ReasonCode = "output_limit"
	ReasonInvalidInventory     ReasonCode = "invalid_inventory"
	ReasonLegacyErrors         ReasonCode = "legacy_errors"
	ReasonRuntimeUnsupported   ReasonCode = "runtime_unsupported"
	ReasonUnsupportedLaunch    ReasonCode = "unsupported_launch_context"
	ReasonMetadataUnavailable  ReasonCode = "metadata_unavailable"
	ReasonAmbiguousIdentity    ReasonCode = "ambiguous_identity"
	ReasonMissingRoot          ReasonCode = "missing_root"
	ReasonManifestMismatch     ReasonCode = "manifest_mismatch"
	ReasonUnsafeRoot           ReasonCode = "unsafe_root"
	ReasonBuiltinNotMountable  ReasonCode = "builtin_not_mountable"
	ReasonExternalNotMountable ReasonCode = "external_not_mountable"
	ReasonUnsupportedSource    ReasonCode = "unsupported_source"
	ReasonPluginMountConflict  ReasonCode = "manual_mount_conflict"
	ReasonPolicyUnverified     ReasonCode = "policy_unverified"
)

// Availability is the per-row selectability state.
type Availability string

const (
	AvailabilityAvailable   Availability = "available"
	AvailabilityUnavailable Availability = "unavailable"
)

const (
	inventoryMaxRows     = 1024
	inventoryFormatFlat  = "flat_array"
	inventoryFormatEnvel = "legacy_envelope"
)

// InventoryRow is one safe, bounded plugin row. Root is internal evidence and
// is never projected into a browser DTO or log.
type InventoryRow struct {
	ID            PluginID
	Name          string
	Marketplace   string
	Version       string
	NativeEnabled bool
	Source        string
	InstalledFrom string
	Availability  Availability
	Reason        ReasonCode

	root string
}

// Snapshot is the parsed inventory plus its safe runtime identity.
type Snapshot struct {
	Status         Status
	Reason         ReasonCode
	Rows           []InventoryRow
	RuntimeVersion string
	Platform       string
	Format         string
}

// LaunchContext is the authorized final process context used to resolve
// inventory and plugin directories.
type LaunchContext struct {
	AgentType      string
	ExecutorType   string
	Env            map[string]string
	WorkDir        string
	CLIFlags       []string
	CommandPrefix  []string
	RuntimeVersion string
	RuntimeCommand []string
	// InventoryTimeout optionally bounds the native inventory command. Zero
	// uses the production deadline.
	InventoryTimeout time.Duration
}

// Discover runs `copilot plugin list --json` under the selected runtime and
// profile environment, then resolves every native row to a safe availability
// state.
func Discover(ctx context.Context, runner Runner, launch LaunchContext) Snapshot {
	snapshot := Snapshot{
		Status:         StatusUnavailable,
		Rows:           []InventoryRow{},
		RuntimeVersion: launch.RuntimeVersion,
		Platform:       runtime.GOOS + "/" + runtime.GOARCH,
	}
	if err := launch.validate(); err != nil {
		snapshot.Status = StatusUnsupported
		snapshot.Reason = reasonForContextError(err)
		return snapshot
	}
	if !RuntimeSupported(launch.RuntimeVersion) {
		snapshot.Status = StatusUnsupported
		snapshot.Reason = ReasonRuntimeUnsupported
		return snapshot
	}
	result, reason := executeInventory(ctx, runner, launch)
	if reason != "" {
		snapshot.Reason = reason
		return snapshot
	}
	rows, format, reason := parseInventory(result.Stdout)
	if reason != "" {
		snapshot.Reason = reason
		return snapshot
	}
	snapshot.Format = format
	snapshot.Rows = rows
	// Resolution failures are per-row availability states, not snapshot
	// failures: a user must still see and remove a broken selection.
	_ = resolveInventory(rows, launch)
	snapshot.Status = StatusReady
	snapshot.Reason = ""
	return snapshot
}

func executeInventory(ctx context.Context, runner Runner, launch LaunchContext) (CommandResult, ReasonCode) {
	command, err := inventoryCommand(launch)
	if err != nil {
		return CommandResult{}, reasonForContextError(err)
	}
	if err := managednpm.PrepareNPMProjectPrefix(command); err != nil {
		return CommandResult{}, ReasonCLIUnavailable
	}
	workDir := launch.WorkDir
	cleanup := func() {}
	if workDir == "" {
		workDir, err = os.MkdirTemp("", "kandev-copilot-inventory-")
		if err != nil {
			return CommandResult{}, ReasonCLIUnavailable
		}
		cleanup = func() { _ = os.RemoveAll(workDir) }
	}
	defer cleanup()

	timeout := inventoryTimeout
	if launch.InventoryTimeout > 0 {
		timeout = launch.InventoryTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, runErr := runner.Run(commandCtx, command, launch.Env, workDir)
	if ctxErr := commandCtx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return result, ReasonTimeout
		}
		return result, ReasonCancelled
	}
	if runErr != nil || result.Truncated || result.ExitCode != 0 {
		if result.Truncated {
			return result, ReasonOutputLimit
		}
		return result, ReasonCLIUnavailable
	}
	return result, ""
}

func inventoryCommand(launch LaunchContext) ([]string, error) {
	configDir, _, err := configDirForLaunch(launch)
	if err != nil {
		return nil, err
	}
	command := append([]string{}, launch.RuntimeCommand...)
	command = append(command, "plugin", "list")
	if configDir != "" {
		command = append(command, "--config-dir="+configDir)
	}
	command = append(command, "--json")
	return command, nil
}

type inventoryRowWire struct {
	Name          *string `json:"name"`
	Marketplace   *string `json:"marketplace"`
	Version       *string `json:"version"`
	Enabled       *bool   `json:"enabled"`
	Source        *string `json:"source"`
	InstalledFrom *string `json:"installedFrom"`
}

func parseInventory(output []byte) ([]InventoryRow, string, ReasonCode) {
	if len(bytes.TrimSpace(output)) == 0 {
		return nil, "", ReasonInvalidInventory
	}
	standardized, err := hujson.Standardize(output)
	if err != nil {
		return nil, "", ReasonInvalidInventory
	}
	if err := rejectDuplicateJSONKeys(standardized); err != nil {
		return nil, "", ReasonInvalidInventory
	}
	trimmed := bytes.TrimSpace(standardized)
	switch trimmed[0] {
	case '[':
		rows, reason := parseInventoryRows(trimmed)
		return rows, inventoryFormatFlat, reason
	case '{':
		return parseInventoryEnvelope(trimmed)
	default:
		return nil, "", ReasonInvalidInventory
	}
}

func parseInventoryEnvelope(data []byte) ([]InventoryRow, string, ReasonCode) {
	var envelope struct {
		Plugins *[]json.RawMessage `json:"plugins"`
		Errors  *[]json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Plugins == nil || envelope.Errors == nil {
		return nil, "", ReasonInvalidInventory
	}
	if len(*envelope.Errors) > 0 {
		return nil, inventoryFormatEnvel, ReasonLegacyErrors
	}
	rows := make([]InventoryRow, 0, len(*envelope.Plugins))
	for _, raw := range *envelope.Plugins {
		row, reason := parseInventoryRow(raw)
		if reason != "" {
			return nil, inventoryFormatEnvel, reason
		}
		rows = append(rows, row)
	}
	if len(rows) > inventoryMaxRows {
		return nil, inventoryFormatEnvel, ReasonInvalidInventory
	}
	return dedupeInventoryRows(rows), inventoryFormatEnvel, ""
}

func parseInventoryRows(data []byte) ([]InventoryRow, ReasonCode) {
	var rawRows []json.RawMessage
	if err := json.Unmarshal(data, &rawRows); err != nil {
		return nil, ReasonInvalidInventory
	}
	if len(rawRows) > inventoryMaxRows {
		return nil, ReasonInvalidInventory
	}
	rows := make([]InventoryRow, 0, len(rawRows))
	for _, raw := range rawRows {
		row, reason := parseInventoryRow(raw)
		if reason != "" {
			return nil, reason
		}
		rows = append(rows, row)
	}
	return dedupeInventoryRows(rows), ""
}

func parseInventoryRow(raw []byte) (InventoryRow, ReasonCode) {
	var wire inventoryRowWire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&wire); err != nil {
		return InventoryRow{}, ReasonInvalidInventory
	}
	if wire.Name == nil || wire.Enabled == nil || wire.Source == nil {
		return InventoryRow{}, ReasonInvalidInventory
	}
	name, source, marketplace, version, installedFrom, reason := inventoryRowFields(wire)
	if reason != "" {
		return InventoryRow{}, reason
	}
	id, err := NewPluginID(source, marketplace, name)
	if err != nil {
		return InventoryRow{}, ReasonInvalidInventory
	}
	row := InventoryRow{
		ID:            id,
		Name:          name,
		Marketplace:   marketplace,
		Version:       version,
		NativeEnabled: *wire.Enabled,
		Source:        source,
		InstalledFrom: installedFrom,
		Availability:  AvailabilityAvailable,
	}
	switch source {
	case SourceBuiltin:
		row.Availability = AvailabilityUnavailable
		row.Reason = ReasonBuiltinNotMountable
	case SourceExternal:
		row.Availability = AvailabilityUnavailable
		row.Reason = ReasonExternalNotMountable
	case SourceInstalled, SourceLive:
		// Availability is decided by installation evidence.
	default:
		row.Availability = AvailabilityUnavailable
		row.Reason = ReasonUnsupportedSource
	}
	return row, ""
}

func inventoryRowFields(wire inventoryRowWire) (string, string, string, string, string, ReasonCode) {
	name := strings.TrimSpace(*wire.Name)
	source := strings.TrimSpace(*wire.Source)
	if name == "" || len(name) > maxPluginIDBytes || !utf8Safe(name) || !validSourceSegment(source) {
		return "", "", "", "", "", ReasonInvalidInventory
	}
	marketplace := ""
	if wire.Marketplace != nil {
		marketplace = strings.TrimSpace(*wire.Marketplace)
	}
	version := ""
	if wire.Version != nil {
		version = strings.TrimSpace(*wire.Version)
	}
	installedFrom := ""
	if wire.InstalledFrom != nil {
		installedFrom = strings.TrimSpace(*wire.InstalledFrom)
	}
	if len(marketplace) > maxMarketplaceBytes || len(version) > maxPluginIDBytes || len(installedFrom) > maxPluginIDBytes ||
		!utf8Safe(marketplace) || !utf8Safe(version) || !utf8Safe(installedFrom) {
		return "", "", "", "", "", ReasonInvalidInventory
	}
	return name, source, marketplace, version, installedFrom, ""
}

func dedupeInventoryRows(rows []InventoryRow) []InventoryRow {
	seen := make(map[string]int, len(rows))
	result := make([]InventoryRow, 0, len(rows))
	for _, row := range rows {
		id := row.ID.String()
		if index, exists := seen[id]; exists {
			result[index].Availability = AvailabilityUnavailable
			result[index].Reason = ReasonAmbiguousIdentity
			result[index].root = ""
			continue
		}
		seen[id] = len(result)
		result = append(result, row)
	}
	return result
}

func reasonForContextError(err error) ReasonCode {
	if errors.Is(err, errUnsupportedLaunchContext) {
		return ReasonUnsupportedLaunch
	}
	return ReasonUnsupportedLaunch
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := walkJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		return walkJSONObject(decoder)
	case '[':
		return walkJSONArray(decoder)
	default:
		return errors.New("unexpected delimiter")
	}
}

func walkJSONObject(decoder *json.Decoder) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("object key is not a string")
		}
		if _, duplicate := seen[key]; duplicate {
			return errors.New("duplicate object key")
		}
		seen[key] = struct{}{}
		if err := walkJSONValue(decoder); err != nil {
			return err
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return errors.New("object not closed")
	}
	return nil
}

func walkJSONArray(decoder *json.Decoder) error {
	for decoder.More() {
		if err := walkJSONValue(decoder); err != nil {
			return err
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim(']') {
		return errors.New("array not closed")
	}
	return nil
}

func utf8Safe(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r == '\x00' || (r < 0x20 && r != '\t') {
			return false
		}
	}
	return true
}

// DiscoveryIsEmpty reports whether a ready snapshot has no rows at all.
func (s Snapshot) DiscoveryIsEmpty() bool {
	return s.Status == StatusReady && len(s.Rows) == 0
}
