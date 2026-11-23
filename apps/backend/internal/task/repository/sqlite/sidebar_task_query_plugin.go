package sqlite

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

func sidebarPluginGroupingCTEs(driver string, query models.SidebarTaskViewQuery, prefs models.SidebarTaskViewPreferences) (ctes, join, key, label, order string, args []any) {
	grouping := prefs.PluginGrouping
	if grouping == nil {
		return "", "", "", "", "", nil
	}
	type value struct {
		Key       string `json:"key"`
		ID        string `json:"id"`
		Label     string `json:"label"`
		Order     int    `json:"position"`
		sortLabel string
	}
	values := make([]value, len(grouping.Snapshot.Groups))
	for index, group := range grouping.Snapshot.Groups {
		values[index] = value{Key: models.SidebarPluginValueKey(query.Group, group.ID), ID: group.ID, Label: group.Label, sortLabel: strings.ToLower(group.Label)}
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].sortLabel != values[j].sortLabel {
			return values[i].sortLabel < values[j].sortLabel
		}
		return values[i].ID < values[j].ID
	})
	for index := range values {
		values[index].Order = index
	}
	encodedValues, _ := json.Marshal(values)
	encodedMembers, _ := json.Marshal(grouping.Snapshot.Memberships)
	if dialect.IsPostgres(driver) {
		ctes = `, sidebar_plugin_values AS MATERIALIZED (SELECT value.key AS value_key,value.id AS value_id,value.label AS value_label,value.position AS group_order FROM jsonb_to_recordset(CAST(? AS JSONB)) AS value(key text,id text,label text,position integer)), sidebar_plugin_members AS MATERIALIZED (SELECT DISTINCT task_id,group_id FROM jsonb_to_recordset(CAST(? AS JSONB)) AS member(task_id text,group_id text))`
	} else {
		ctes = `, sidebar_plugin_values AS MATERIALIZED (SELECT json_extract(value,'$.key') AS value_key,json_extract(value,'$.id') AS value_id,json_extract(value,'$.label') AS value_label,json_extract(value,'$.position') AS group_order FROM json_each(?)), sidebar_plugin_members AS MATERIALIZED (SELECT DISTINCT json_extract(value,'$.task_id') AS task_id,json_extract(value,'$.group_id') AS group_id FROM json_each(?))`
	}
	join = ` LEFT JOIN sidebar_plugin_members plugin_member ON plugin_member.task_id=v.id LEFT JOIN sidebar_plugin_values plugin_value ON plugin_value.value_id=plugin_member.group_id`
	key = `COALESCE(plugin_value.value_key,'` + models.SidebarPluginUnassignedKey(query.Group) + `')`
	label = `COALESCE(plugin_value.value_label,'')`
	order = `COALESCE((SELECT value.group_order FROM sidebar_plugin_values value WHERE value.value_key=root_groups.group_key),1001), group_key`
	args = []any{string(encodedValues), string(encodedMembers)}
	return ctes, join, key, label, order, args
}

func validateSidebarPluginGrouping(query models.SidebarTaskViewQuery, prefs models.SidebarTaskViewPreferences) error {
	_, _, plugin := pluginsdk.ParseSidebarGroupingReference(query.Group)
	if !plugin {
		return nil
	}
	grouping := prefs.PluginGrouping
	if grouping == nil || grouping.Reference != query.Group || grouping.Generation == "" || grouping.Snapshot == nil || !grouping.Snapshot.Complete || grouping.Snapshot.Version != 1 || grouping.Snapshot.Revision == "" {
		return fmt.Errorf("sidebar grouping unavailable")
	}
	return nil
}

func projectSidebarPluginGrouping(result *models.SidebarTaskPageResult, grouping *models.SidebarPluginGrouping) {
	if grouping == nil {
		return
	}
	result.GroupingRevision = grouping.Snapshot.Revision
	result.GroupingGeneration = grouping.Generation
	colors := make(map[string]string, len(grouping.Snapshot.Groups))
	for _, value := range grouping.Snapshot.Groups {
		colors[models.SidebarPluginValueKey(grouping.Reference, value.ID)] = value.Color
	}
	for index := range result.Entries {
		result.Entries[index].GroupColor = colors[result.Entries[index].GroupKey]
	}
}
