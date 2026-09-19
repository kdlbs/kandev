package service

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/user/models"
)

const (
	maxSidebarLayoutNodes          = 40
	maxSidebarLayoutShortcutsGroup = 20
	maxSidebarLayoutShortcutsTotal = 100
	maxSidebarLayoutIDRunes        = 255
	maxSidebarLayoutBytes          = 256 * 1024
	maxSidebarLayoutNameRunes      = 60
)

// validateSidebarLayoutPatch checks the request shape and the caller's
// workspace scope before the settings CAS starts. The layout itself remains
// an opaque, complete replacement inside the chosen workspace.
func (s *Service) validateSidebarLayoutPatch(
	ctx context.Context,
	req *UpdateUserSettingsRequest,
	provided ...[]string,
) error {
	patch := req.SidebarLayoutState
	if patch == nil {
		return nil
	}
	if s.sidebarWorkspaceAccess == nil {
		return fmt.Errorf("%w: workspace access unavailable", ErrValidation)
	}
	workspaceID := strings.TrimSpace(patch.WorkspaceID)
	if workspaceID == "" {
		return fmt.Errorf("%w: workspace is required", ErrValidation)
	}
	ids, err := s.resolveSidebarWorkspaceIDs(ctx, provided...)
	if err != nil {
		return err
	}
	if !slices.Contains(ids, workspaceID) {
		return fmt.Errorf("%w: invalid sidebar layout workspace", ErrValidation)
	}
	if patch.ExpectedRevision < 0 {
		return fmt.Errorf("%w: sidebar layout revision must be non-negative", ErrValidation)
	}
	if patch.Layout == nil {
		return nil
	}
	return validateSidebarLayout(*patch.Layout)
}

func validateSidebarLayout(layout models.SidebarLayout) error {
	if layout.Version != models.SidebarLayoutVersion {
		return fmt.Errorf("sidebar layout version %d is unsupported", layout.Version)
	}
	if len(layout.Nodes) > maxSidebarLayoutNodes {
		return fmt.Errorf("sidebar layout has more than %d nodes", maxSidebarLayoutNodes)
	}

	nodeIDs := make(map[string]struct{}, len(layout.Nodes))
	shortcutIDs := make(map[string]struct{})
	totalShortcuts := 0
	for _, node := range layout.Nodes {
		shortcuts, err := validateSidebarLayoutNode(node, nodeIDs, shortcutIDs)
		if err != nil {
			return err
		}
		totalShortcuts += shortcuts
	}
	if totalShortcuts > maxSidebarLayoutShortcutsTotal {
		return fmt.Errorf("sidebar layout has more than %d shortcuts", maxSidebarLayoutShortcutsTotal)
	}
	raw, err := json.Marshal(layout)
	if err != nil {
		return fmt.Errorf("sidebar layout is not serializable: %w", err)
	}
	if len(raw) > maxSidebarLayoutBytes {
		return fmt.Errorf("sidebar layout exceeds %d bytes", maxSidebarLayoutBytes)
	}
	return nil
}

func validateSidebarLayoutNode(
	node models.SidebarLayoutNode,
	nodeIDs, shortcutIDs map[string]struct{},
) (int, error) {
	if err := validateSidebarLayoutID("node", node.ID); err != nil {
		return 0, err
	}
	if _, exists := nodeIDs[node.ID]; exists {
		return 0, fmt.Errorf("sidebar layout node %q is duplicated", node.ID)
	}
	nodeIDs[node.ID] = struct{}{}

	switch node.Kind {
	case models.SidebarLayoutNodeBuiltin, models.SidebarLayoutNodePlugin:
		return 0, validateSidebarDestinationNode(node)
	case models.SidebarLayoutNodeShortcuts:
		return len(node.Shortcuts), validateSidebarShortcutSection(node, shortcutIDs)
	default:
		return 0, fmt.Errorf("sidebar layout node %q has unsupported kind %q", node.ID, node.Kind)
	}
}

func validateSidebarDestinationNode(node models.SidebarLayoutNode) error {
	if strings.TrimSpace(node.DestinationID) == "" {
		return fmt.Errorf("sidebar layout node %q needs a destination", node.ID)
	}
	if node.Kind == models.SidebarLayoutNodeBuiltin && isProtectedSidebarDestination(node.DestinationID) {
		return fmt.Errorf("sidebar destination %q is fixed", node.DestinationID)
	}
	if node.Name != "" || len(node.Shortcuts) != 0 {
		return fmt.Errorf("sidebar destination node %q has shortcut-section fields", node.ID)
	}
	return nil
}

func validateSidebarShortcutSection(node models.SidebarLayoutNode, shortcutIDs map[string]struct{}) error {
	name := strings.TrimSpace(node.Name)
	if name == "" || utf8.RuneCountInString(name) > maxSidebarLayoutNameRunes {
		return fmt.Errorf("sidebar shortcut section %q must have 1-%d characters", node.ID, maxSidebarLayoutNameRunes)
	}
	if node.DestinationID != "" {
		return fmt.Errorf("sidebar shortcut section %q has a destination", node.ID)
	}
	if len(node.Shortcuts) > maxSidebarLayoutShortcutsGroup {
		return fmt.Errorf("sidebar shortcut section %q has more than %d shortcuts", node.ID, maxSidebarLayoutShortcutsGroup)
	}
	targets := make(map[string]struct{}, len(node.Shortcuts))
	for _, shortcut := range node.Shortcuts {
		if err := validateSidebarLayoutID("shortcut", shortcut.ID); err != nil {
			return err
		}
		if _, exists := shortcutIDs[shortcut.ID]; exists {
			return fmt.Errorf("sidebar shortcut %q is duplicated", shortcut.ID)
		}
		shortcutIDs[shortcut.ID] = struct{}{}
		if err := validateSidebarShortcutTarget(shortcut.Target); err != nil {
			return err
		}
		targetKey := shortcut.Target.Kind + ":" + shortcut.Target.ID
		if _, exists := targets[targetKey]; exists {
			return fmt.Errorf("sidebar shortcut target %q is duplicated in section %q", targetKey, node.ID)
		}
		targets[targetKey] = struct{}{}
	}
	return nil
}

func validateSidebarLayoutID(kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("sidebar %s id is required", kind)
	}
	if utf8.RuneCountInString(value) > maxSidebarLayoutIDRunes {
		return fmt.Errorf("sidebar %s id exceeds %d characters", kind, maxSidebarLayoutIDRunes)
	}
	return nil
}

func validateSidebarShortcutTarget(target models.SidebarShortcutTarget) error {
	if strings.TrimSpace(target.ID) == "" {
		return fmt.Errorf("sidebar shortcut target id is required")
	}
	switch target.Kind {
	case models.SidebarShortcutDestination, models.SidebarShortcutCanvas, models.SidebarShortcutAutomation:
		return nil
	case models.SidebarShortcutHostAction:
		switch target.ID {
		case "new_task", "quick_chat", "quick_terminal":
			return nil
		default:
			return fmt.Errorf("sidebar host action %q is unsupported", target.ID)
		}
	default:
		return fmt.Errorf("sidebar shortcut target kind %q is unsupported", target.Kind)
	}
}

func isProtectedSidebarDestination(id string) bool {
	switch id {
	case "tasks", "inbox", "needs_you_inbox":
		return true
	default:
		return false
	}
}

func applySidebarLayoutPatch(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	patch := req.SidebarLayoutState
	if patch == nil {
		return nil
	}
	workspaceID := strings.TrimSpace(patch.WorkspaceID)
	current, exists := settings.SidebarLayoutsByWorkspace[workspaceID]
	if !exists {
		current = models.DefaultSidebarLayout()
	} else if current.Revision < 0 {
		current.Revision = 0
	}
	if current.Revision != patch.ExpectedRevision {
		return ErrUserSettingsConflict
	}

	next := models.DefaultSidebarLayout()
	if patch.Layout != nil {
		next = *patch.Layout
	}
	next.Version = models.SidebarLayoutVersion
	next.Revision = patch.ExpectedRevision + 1
	if next.Nodes == nil {
		next.Nodes = []models.SidebarLayoutNode{}
	}
	entries := maps.Clone(settings.SidebarLayoutsByWorkspace)
	if entries == nil {
		entries = map[string]models.SidebarLayout{}
	}
	entries[workspaceID] = next
	settings.SidebarLayoutsByWorkspace = entries
	return nil
}

func projectSidebarLayouts(settings *models.UserSettings, ids []string) map[string]models.SidebarLayout {
	projected := make(map[string]models.SidebarLayout, len(ids))
	for _, id := range ids {
		layout, exists := settings.SidebarLayoutsByWorkspace[id]
		switch {
		case !exists:
			layout = models.DefaultSidebarLayout()
		case layout.Version != models.SidebarLayoutVersion:
			fallback := models.DefaultSidebarLayout()
			fallback.Revision = maxInt64(layout.Revision, 0)
			layout = fallback
		case layout.Nodes == nil:
			layout.Nodes = []models.SidebarLayoutNode{}
		}
		projected[id] = layout
	}
	return projected
}

func maxInt64(value, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}
