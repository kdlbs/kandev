package pluginsdk

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxSidebarGroupingResponseBytes = 1 << 20
	MaxSidebarGroupingValues        = 1000
	MaxSidebarGroupingMemberships   = 10000
)

var sidebarGroupingColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var errInvalidSidebarGroupingSnapshot = errors.New("invalid sidebar grouping snapshot")

// SidebarGroupingSnapshot is a complete workspace classification returned by a
// manifest-declared sidebar grouping action. Membership never grants access.
type SidebarGroupingSnapshot struct {
	Version     int                         `json:"version"`
	Revision    string                      `json:"revision"`
	Complete    bool                        `json:"complete"`
	Groups      []SidebarGroupingValue      `json:"groups"`
	Memberships []SidebarGroupingMembership `json:"memberships"`
}

type SidebarGroupingValue struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color,omitempty"`
}

type SidebarGroupingMembership struct {
	TaskID  string `json:"task_id"`
	GroupID string `json:"group_id"`
}

// DecodeSidebarGroupingSnapshot validates a bounded, explicitly complete snapshot.
func DecodeSidebarGroupingSnapshot(body []byte) (*SidebarGroupingSnapshot, error) {
	invalid := errInvalidSidebarGroupingSnapshot
	if len(body) > MaxSidebarGroupingResponseBytes || !utf8.Valid(body) {
		return nil, invalid
	}
	var snapshot SidebarGroupingSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return nil, invalid
	}
	if snapshot.Version != 1 || !snapshot.Complete || !sidebarGroupingText(snapshot.Revision, 256) ||
		snapshot.Groups == nil || snapshot.Memberships == nil ||
		len(snapshot.Groups) > MaxSidebarGroupingValues || len(snapshot.Memberships) > MaxSidebarGroupingMemberships {
		return nil, invalid
	}
	values := make(map[string]struct{}, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		if !sidebarGroupingText(group.ID, 256) || !sidebarGroupingText(group.Label, 256) ||
			(group.Color != "" && !sidebarGroupingColor.MatchString(group.Color)) {
			return nil, invalid
		}
		if _, exists := values[group.ID]; exists {
			return nil, invalid
		}
		values[group.ID] = struct{}{}
	}
	seen := make(map[SidebarGroupingMembership]struct{}, len(snapshot.Memberships))
	pairs := snapshot.Memberships[:0]
	for _, pair := range snapshot.Memberships {
		if !sidebarGroupingText(pair.TaskID, 256) {
			return nil, invalid
		}
		if _, exists := values[pair.GroupID]; !exists {
			return nil, invalid
		}
		if _, duplicate := seen[pair]; duplicate {
			continue
		}
		seen[pair] = struct{}{}
		pairs = append(pairs, pair)
	}
	snapshot.Memberships = pairs
	return &snapshot, nil
}

func sidebarGroupingText(value string, maxBytes int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maxBytes && utf8.ValidString(value) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}
