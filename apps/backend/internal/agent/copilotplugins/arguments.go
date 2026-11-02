package copilotplugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

// Error is a bounded, user-safe plugin preparation failure. It never carries
// a resolved root, native output, environment value, or credential.
type Error struct {
	Reason   ReasonCode
	PluginID string
}

func (e *Error) Error() string {
	action := repairAction(e.Reason)
	if e.PluginID != "" {
		return fmt.Sprintf("copilot plugin %q is unavailable (%s). %s", e.PluginID, e.Reason, action)
	}
	return fmt.Sprintf("copilot plugins are unavailable (%s). %s", e.Reason, action)
}

// IsUnsupported reports whether the failure is a configuration limitation that
// the user can repair by changing the profile selection, executor, or runtime.
func (e *Error) IsUnsupported() bool {
	switch e.Reason {
	case ReasonRuntimeUnsupported, ReasonUnsupportedLaunch, ReasonUnsupportedSource,
		ReasonBuiltinNotMountable, ReasonExternalNotMountable, ReasonPluginMountConflict, ReasonPolicyUnverified:
		return true
	default:
		return false
	}
}

func repairAction(reason ReasonCode) string {
	switch reason {
	case ReasonRuntimeUnsupported:
		return "Select a supported Copilot runtime in the profile's runtime settings."
	case ReasonPolicyUnverified:
		return "Uncheck the selected plugins and save the profile to start without them."
	case ReasonUnsupportedLaunch:
		return "Use a local or worktree executor with a verified configuration directory."
	case ReasonBuiltinNotMountable:
		return "Built-in plugins cannot be loaded by directory. Remove it from the profile."
	case ReasonExternalNotMountable:
		return "This plugin is loaded by a profile flag. Remove the selection or the manual flag."
	case ReasonUnsupportedSource:
		return "Remove the plugin or reinstall it from a supported marketplace."
	case ReasonPluginMountConflict:
		return "Remove the profile's manual plugin directory flag or uncheck the selected plugin."
	case ReasonMissingRoot, ReasonMetadataUnavailable, ReasonManifestMismatch, ReasonUnsafeRoot, ReasonAmbiguousIdentity:
		return "Refresh plugins in the profile, repair the install, or uncheck it."
	case ReasonTimeout, ReasonCLIUnavailable:
		return "Run Copilot's plugin list for this profile and try again."
	case ReasonInvalidInventory, ReasonLegacyErrors, ReasonOutputLimit:
		return "Update Copilot or repair its plugin state, then try again."
	default:
		return "Refresh plugins in the profile or uncheck the selection."
	}
}

// ResolvedPlugin is one selected identity and its verified directory. Root is
// internal and never projected.
type ResolvedPlugin struct {
	ID   PluginID
	Root string
}

// Prepared is the deterministic native mount preparation for one new process.
type Prepared struct {
	Plugins []ResolvedPlugin
}

// ResolveLaunchSelection is the production admission boundary. It validates
// the final context, preserves empty-selection behavior, and rejects runtimes
// without a supported inventory and metadata adapter.
func ResolveLaunchSelection(ctx context.Context, runner Runner, launch LaunchContext, selection []string) (Prepared, error) {
	if len(selection) == 0 {
		return Prepared{}, nil
	}
	if err := launch.validate(); err != nil {
		return Prepared{}, &Error{Reason: reasonForContextError(err)}
	}
	if !RuntimeSupported(launch.RuntimeVersion) {
		return Prepared{}, &Error{Reason: ReasonRuntimeUnsupported}
	}
	return ResolveSelection(ctx, runner, launch, selection)
}

// ResolveSelection turns stable selections into verified native roots for one
// new process. It is the low-level adapter used after policy admission. An
// empty selection is a no-op and never runs the native CLI.
func ResolveSelection(ctx context.Context, runner Runner, launch LaunchContext, selection []string) (Prepared, error) {
	if len(selection) == 0 {
		return Prepared{}, nil
	}
	if err := launch.validate(); err != nil {
		return Prepared{}, &Error{Reason: reasonForContextError(err)}
	}
	normalized, err := NormalizeSelection(selection)
	if err != nil {
		return Prepared{}, &Error{Reason: ReasonMetadataUnavailable}
	}
	snapshot := Discover(ctx, runner, launch)
	if snapshot.Status == StatusUnsupported {
		return Prepared{}, &Error{Reason: snapshot.Reason}
	}
	if snapshot.Status != StatusReady {
		return Prepared{}, &Error{Reason: snapshot.Reason}
	}
	rows := make(map[string]InventoryRow, len(snapshot.Rows))
	for _, row := range snapshot.Rows {
		rows[row.ID.String()] = row
	}
	resolved := make([]ResolvedPlugin, 0, len(normalized))
	for _, id := range normalized {
		row, ok := rows[id]
		if !ok {
			return Prepared{}, &Error{Reason: ReasonMetadataUnavailable, PluginID: id}
		}
		if row.Availability != AvailabilityAvailable || row.root == "" {
			return Prepared{}, &Error{Reason: row.Reason, PluginID: id}
		}
		resolved = append(resolved, ResolvedPlugin{ID: row.ID, Root: row.root})
	}
	sort.Slice(resolved, func(i, j int) bool {
		return resolved[i].ID.String() < resolved[j].ID.String()
	})
	return Prepared{Plugins: resolved}, nil
}

// ApplyToArgs appends one `--plugin-dir`/root pair per selected plugin before
// any end-of-options delimiter. Manual profile mounts keep their order and
// meaning. Equal roots are not duplicated; a manual mount whose manifest names
// the same plugin at a different root is a visible conflict.
func ApplyToArgs(baseArgs []string, plugins []ResolvedPlugin) ([]string, error) {
	return ApplyToArgsInDir(baseArgs, plugins, "")
}

// ApplyToArgsInDir is ApplyToArgs with the session working directory used to
// resolve relative manual --plugin-dir values, matching Copilot's own
// resolution rules.
func ApplyToArgsInDir(baseArgs []string, plugins []ResolvedPlugin, workDir string) ([]string, error) {
	args := append([]string{}, baseArgs...)
	if len(plugins) == 0 {
		return args, nil
	}
	manual, err := parseManualPluginDirs(args, workDir)
	if err != nil {
		return nil, err
	}
	manualByRoot := make(map[string]manualMount, len(manual))
	manualByName := make(map[string]manualMount, len(manual))
	for _, mount := range manual {
		manualByRoot[mount.canonicalRoot] = mount
		if mount.name != "" {
			manualByName[mount.name] = mount
		}
	}
	additions := make([]string, 0, len(plugins)*2)
	for _, plugin := range plugins {
		root := canonicalRoot(plugin.Root, "")
		if root == "" {
			return nil, &Error{Reason: ReasonUnsafeRoot, PluginID: plugin.ID.String()}
		}
		if _, exists := manualByRoot[root]; exists {
			continue
		}
		if mount, exists := manualByName[plugin.ID.Name]; exists && mount.canonicalRoot != root {
			return nil, &Error{Reason: ReasonPluginMountConflict, PluginID: plugin.ID.String()}
		}
		additions = append(additions, "--plugin-dir", plugin.Root)
	}
	if len(additions) == 0 {
		return args, nil
	}
	insertAt := len(args)
	for index, arg := range args {
		if arg == "--" {
			insertAt = index
			break
		}
	}
	result := make([]string, 0, len(args)+len(additions))
	result = append(result, args[:insertAt]...)
	result = append(result, additions...)
	result = append(result, args[insertAt:]...)
	return result, nil
}

type manualMount struct {
	value         string
	canonicalRoot string
	name          string
}

func parseManualPluginDirs(args []string, workDir string) ([]manualMount, error) {
	mounts := make([]manualMount, 0)
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			break
		}
		value := ""
		switch {
		case arg == "--plugin-dir":
			if index+1 >= len(args) || args[index+1] == "" {
				return nil, &Error{Reason: ReasonUnsupportedLaunch}
			}
			index++
			value = args[index]
		case strings.HasPrefix(arg, "--plugin-dir="):
			value = strings.TrimPrefix(arg, "--plugin-dir=")
			if value == "" {
				return nil, &Error{Reason: ReasonUnsupportedLaunch}
			}
		default:
			continue
		}
		mount := manualMount{value: value, canonicalRoot: canonicalRoot(value, workDir)}
		resolved := value
		if !filepath.IsAbs(resolved) && workDir != "" {
			resolved = filepath.Join(workDir, resolved)
		}
		if name, ok := manifestName(resolved); ok {
			mount.name = name
		}
		mounts = append(mounts, mount)
	}
	return mounts, nil
}

func canonicalRoot(root, workDir string) string {
	if root == "" || strings.ContainsRune(root, '\x00') {
		return ""
	}
	absolute := root
	if !filepath.IsAbs(absolute) && workDir != "" {
		absolute = filepath.Join(workDir, absolute)
	}
	absolute, err := filepath.Abs(absolute)
	if err != nil {
		return ""
	}
	clean := filepath.Clean(absolute)
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		return resolved
	}
	return clean
}

func manifestName(root string) (string, bool) {
	manifestPath, found := firstExistingManifest(root)
	if !found {
		return "", false
	}
	info, err := os.Lstat(manifestPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", false
	}
	data, err := readBoundedFile(manifestPath, maxPluginManifestBytes)
	if err != nil {
		return "", false
	}
	standardized, err := hujson.Standardize(data)
	if err != nil {
		return "", false
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(standardized, &manifest); err != nil || strings.TrimSpace(manifest.Name) == "" {
		return "", false
	}
	return strings.TrimSpace(manifest.Name), true
}

// IsError reports whether err is a typed plugin preparation failure.
func IsError(err error) bool {
	var target *Error
	return errors.As(err, &target)
}
