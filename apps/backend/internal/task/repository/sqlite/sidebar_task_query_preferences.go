package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/task/models"
)

const sidebarPreferenceTable = "temp.kandev_sidebar_preferences"

// Preference rows keep statement preparation independent of saved-list length.
func (s *sidebarQuerySnapshot) preparePreferences(ctx context.Context, prefs models.SidebarTaskViewPreferences) error {
	if !s.sqlite {
		return nil
	}
	rows := sidebarPreferenceRows(prefs)
	if len(rows) == 0 {
		return nil
	}
	if _, err := s.tx.ExecContext(ctx, "CREATE TEMP TABLE "+sidebarPreferenceTable+` (
  kind TEXT NOT NULL, parent_id TEXT NOT NULL, task_id TEXT NOT NULL, position INTEGER NOT NULL,
  PRIMARY KEY (kind, parent_id, task_id)
 ) WITHOUT ROWID`); err != nil {
		return fmt.Errorf("stage sidebar preference schema: %w", err)
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("encode sidebar preferences: %w", err)
	}
	_, err = s.tx.ExecContext(ctx, s.tx.Rebind("INSERT INTO "+sidebarPreferenceTable+`
 SELECT json_extract(value, '$[0]'), json_extract(value, '$[1]'),
  json_extract(value, '$[2]'), json_extract(value, '$[3]') FROM json_each(?) WHERE 1=1
 ON CONFLICT (kind, parent_id, task_id) DO NOTHING`), string(payload))
	if err != nil {
		return fmt.Errorf("stage sidebar preferences: %w", err)
	}
	return s.checkpoint("preferences")
}

func sidebarPreferenceRows(prefs models.SidebarTaskViewPreferences) [][4]any {
	rows := make([][4]any, 0, len(prefs.PinnedTaskIDs)+len(prefs.OrderedTaskIDs))
	appendIDs := func(kind, parent string, ids []string) {
		for index, id := range ids {
			rows = append(rows, [4]any{kind, parent, id, index})
		}
	}
	appendIDs("pinned", "", prefs.PinnedTaskIDs)
	appendIDs("ordered", "", prefs.OrderedTaskIDs)
	parents := make([]string, 0, len(prefs.SubtaskOrderByParentID))
	for parent := range prefs.SubtaskOrderByParentID {
		parents = append(parents, parent)
	}
	sort.Strings(parents)
	for _, parent := range parents {
		appendIDs("child", parent, prefs.SubtaskOrderByParentID[parent])
	}
	return rows
}

func sidebarPreferenceOrder(kind, parent string) string {
	return "(SELECT position FROM " + sidebarPreferenceTable + " WHERE kind = '" + kind +
		"' AND parent_id = " + parent + " AND task_id = v.id)"
}

func sqliteSidebarIDOrder(kind string, count int) string {
	return "COALESCE(" + sidebarPreferenceOrder(kind, "''") + ", " + fmt.Sprint(count) + ")"
}
