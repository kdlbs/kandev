package pluginsdk

import (
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
	if len(body) > MaxSidebarGroupingResponseBytes || !utf8.Valid(body) {
		return nil, errInvalidSidebarGroupingSnapshot
	}
	snapshot, err := decodeSidebarGroupingJSON(body)
	if err != nil || !validSidebarGroupingEnvelope(&snapshot) {
		return nil, errInvalidSidebarGroupingSnapshot
	}
	values, ok := sidebarGroupingValueIDs(snapshot.Groups)
	if !ok {
		return nil, errInvalidSidebarGroupingSnapshot
	}
	pairs, ok := dedupeSidebarGroupingMemberships(snapshot.Memberships, values)
	if !ok {
		return nil, errInvalidSidebarGroupingSnapshot
	}
	snapshot.Memberships = pairs
	return &snapshot, nil
}

func validSidebarGroupingEnvelope(snapshot *SidebarGroupingSnapshot) bool {
	return snapshot.Version == 1 && snapshot.Complete && sidebarGroupingText(snapshot.Revision, 256) &&
		snapshot.Groups != nil && snapshot.Memberships != nil &&
		len(snapshot.Groups) <= MaxSidebarGroupingValues && len(snapshot.Memberships) <= MaxSidebarGroupingMemberships
}

func sidebarGroupingValueIDs(groups []SidebarGroupingValue) (map[string]struct{}, bool) {
	values := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if !sidebarGroupingText(group.ID, 256) || !sidebarGroupingText(group.Label, 256) ||
			(group.Color != "" && !sidebarGroupingColor.MatchString(group.Color)) {
			return nil, false
		}
		if _, exists := values[group.ID]; exists {
			return nil, false
		}
		values[group.ID] = struct{}{}
	}
	return values, true
}

func dedupeSidebarGroupingMemberships(memberships []SidebarGroupingMembership, values map[string]struct{}) ([]SidebarGroupingMembership, bool) {
	seen := make(map[SidebarGroupingMembership]struct{}, len(memberships))
	pairs := memberships[:0]
	for _, pair := range memberships {
		if !sidebarGroupingText(pair.TaskID, 256) {
			return nil, false
		}
		if _, exists := values[pair.GroupID]; !exists {
			return nil, false
		}
		if _, duplicate := seen[pair]; duplicate {
			continue
		}
		seen[pair] = struct{}{}
		pairs = append(pairs, pair)
	}
	return pairs, true
}

func sidebarGroupingText(value string, maxBytes int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maxBytes && utf8.ValidString(value) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}
