package pluginsdk

import (
	"regexp"
	"strings"
)

var sidebarGroupingIdentity = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

// ParseSidebarGroupingReference accepts a persisted provider identity without
// consulting the live plugin registry. Disabled providers retain their views.
func ParseSidebarGroupingReference(value string) (pluginID, groupingID string, ok bool) {
	rest, prefixed := strings.CutPrefix(value, "plugin:")
	if !prefixed {
		return "", "", false
	}
	pluginID, groupingID, ok = strings.Cut(rest, ":")
	if !ok || !sidebarGroupingIdentity.MatchString(pluginID) || !sidebarGroupingIdentity.MatchString(groupingID) {
		return "", "", false
	}
	return pluginID, groupingID, true
}
