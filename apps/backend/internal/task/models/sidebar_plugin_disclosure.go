package models

import (
	"crypto/sha256"
	"encoding/hex"
)

// SidebarPluginValueKey excludes mutable labels and frames the complete provider
// reference with NUL, which valid provider references cannot contain.
func SidebarPluginValueKey(reference, value string) string {
	digest := sha256.New()
	digest.Write([]byte(reference))
	digest.Write([]byte{0})
	digest.Write([]byte(value))
	var buffer [sha256.Size]byte
	sum := digest.Sum(buffer[:0])
	const prefix = "plugin-group:"
	var key [len(prefix) + 2*sha256.Size]byte
	copy(key[:], prefix)
	hex.Encode(key[len(prefix):], sum)
	return string(key[:])
}

// SidebarPluginUnassignedKey returns the host-owned key for missing values.
func SidebarPluginUnassignedKey(reference string) string {
	return reference + ":unassigned"
}

// SidebarPluginDisclosure intersects preferences with the complete provider
// snapshot, never just the groups present in a bounded task page.
func SidebarPluginDisclosure(grouping *SidebarPluginGrouping, collapsed []string) (current, kept []string) {
	current = make([]string, 0, len(grouping.Snapshot.Groups)+1)
	unassignedKey := SidebarPluginUnassignedKey(grouping.Reference)
	current = append(current, unassignedKey)
	for _, value := range grouping.Snapshot.Groups {
		current = append(current, SidebarPluginValueKey(grouping.Reference, value.ID))
	}
	kept = make([]string, 0, min(len(collapsed), len(current)))
	if len(collapsed) == 0 {
		return current, kept
	}
	allowed := make(map[string]bool, len(current))
	for _, key := range current {
		allowed[key] = true
	}
	for _, key := range collapsed {
		if key == grouping.Reference+":untagged" {
			key = unassignedKey
		}
		if allowed[key] {
			kept = append(kept, key)
			allowed[key] = false
		}
	}
	return current, kept
}
