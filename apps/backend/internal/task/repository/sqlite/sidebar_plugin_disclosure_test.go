package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestSidebarPluginDisclosureFitsPublicQueryBudget(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "ws")
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "root", WorkspaceID: "ws", Title: "Root"}))
	reference := "plugin:" + strings.Repeat("p", 128) + ":" + strings.Repeat("g", 128)
	snapshot := &pluginsdk.SidebarGroupingSnapshot{Version: 1, Complete: true, Revision: "r1"}
	query := sidebarTaskQuery(1)
	query.Group = reference
	for i := range 260 {
		id := strings.Repeat("界", 84) + fmt.Sprintf("%03d", i)
		snapshot.Groups = append(snapshot.Groups, pluginsdk.SidebarGroupingValue{ID: id, Label: fmt.Sprint(i)})
		snapshot.Memberships = append(snapshot.Memberships, pluginsdk.SidebarGroupingMembership{TaskID: "root", GroupID: id})
		query.CollapsedGroupKeys = append(query.CollapsedGroupKeys, models.SidebarPluginValueKey(reference, id))
	}
	wire, err := json.Marshal(query)
	require.NoError(t, err)
	require.LessOrEqual(t, len(wire), 256*1024, "valid persisted disclosure must fit the public sidebar request cap")
	var restored models.SidebarTaskViewQuery
	require.NoError(t, json.Unmarshal(wire, &restored))
	require.NoError(t, restored.Validate())
	prefs := models.SidebarTaskViewPreferences{PluginGrouping: &models.SidebarPluginGrouping{Reference: reference, Generation: "g", Snapshot: snapshot}}
	page, err := repo.QuerySidebarTaskPage(t.Context(), "ws", restored, prefs)
	require.NoError(t, err)
	require.Zero(t, page.TotalVisibleTasks)
	for i := range snapshot.Groups {
		snapshot.Groups[i].Label = "Renamed"
	}
	snapshot.Revision = "r2"
	restored.CollapsedGroupKeys = restored.CollapsedGroupKeys[1:]
	reopened, err := repo.QuerySidebarTaskPage(t.Context(), "ws", restored, prefs)
	require.NoError(t, err)
	require.Equal(t, 1, reopened.TotalVisibleTasks)
	require.NotEqual(t, models.SidebarPluginValueKey(reference, "unassigned"), models.SidebarPluginUnassignedKey(reference))
}
