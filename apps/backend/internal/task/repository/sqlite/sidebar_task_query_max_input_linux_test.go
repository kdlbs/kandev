//go:build linux && cgo

package sqlite

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil/sqlitememory"
	usermodels "github.com/kandev/kandev/internal/user/models"
	"github.com/stretchr/testify/require"
)

func TestSidebarQueryMaximumInputMemory(t *testing.T) {
	if os.Getenv(sidebarMemoryChild) == "" {
		for _, name := range []string{"pinned", "ordered", "children", "filters", "collapsed", "colors"} {
			t.Run(name, func(t *testing.T) {
				t.Setenv(sidebarMemoryCase, name)
				runSidebarMemoryChild(t, "TestSidebarQueryMaximumInputMemory", 101)
			})
		}
		return
	}
	repo, _ := sidebarMemoryFixture(t)
	query, prefs := maximumSidebarInput(t, os.Getenv(sidebarMemoryCase))
	baseline, rss := sqlitememory.Used(), sidebarProcessRSS(t)
	sqlitememory.Peak(true)
	started := time.Now()
	result, err := repo.QuerySidebarTaskPage(t.Context(), "memory-a", query, prefs)
	require.NoError(t, err)
	require.LessOrEqual(t, len(result.Tasks), 100)
	peak := sqlitememory.Peak(false) - baseline
	t.Logf("case=%s native_peak_delta_bytes=%d retained_delta_bytes=%d rss_delta_bytes=%d elapsed=%s", os.Getenv(sidebarMemoryCase), peak, sqlitememory.Used()-baseline, sidebarProcessRSS(t)-rss, time.Since(started))
	require.LessOrEqual(t, peak, 64*sidebarMiB)
	assertSidebarScratchAbsent(t, repo)
}

func maximumSidebarInput(t *testing.T, name string) (models.SidebarTaskViewQuery, models.SidebarTaskViewPreferences) {
	t.Helper()
	query := sidebarTaskQuery(1)
	query.Sort.Key = "custom"
	prefs := models.SidebarTaskViewPreferences{}
	ids := make([]string, models.MaxSidebarViewPreferenceIDs)
	for index := range ids {
		ids[index] = fmt.Sprintf("memory-a-task-%06d", index)
	}
	switch name {
	case "pinned":
		prefs.PinnedTaskIDs = ids
	case "ordered":
		prefs.OrderedTaskIDs = ids
	case "children":
		prefs.SubtaskOrderByParentID = make(map[string][]string)
		for index, id := range ids {
			prefs.SubtaskOrderByParentID[fmt.Sprintf("parent-%d", index)] = []string{id}
		}
	case "collapsed":
		query.CollapsedTaskIDs = ids[:5000]
		query.CollapsedGroupKeys = ids[5000:]
	case "filters":
		values := make([]string, models.MaxSidebarViewListValues)
		for index := range values {
			values[index] = fmt.Sprintf("%04d", index) + strings.Repeat("x", models.MaxSidebarViewValueBytes-4)
		}
		encoded, err := json.Marshal(values)
		require.NoError(t, err)
		for range models.MaxSidebarViewClauses {
			query.Filters = append(query.Filters, models.SidebarTaskViewClause{Dimension: "titleMatch", Op: "not_in", Value: encoded})
		}
	case "colors":
		query.Sort = models.SidebarTaskViewSort{Key: "color", Color: "red", Direction: "desc"}
		manualColors := make(map[string]*string, usermodels.MaxSidebarTaskColors)
		for index := 0; index < usermodels.MaxSidebarTaskColors; index++ {
			manualColors[fmt.Sprintf("color-task-%05d", index)] = strptr("red")
		}
		rules := make([]usermodels.SidebarTaskColorRule, usermodels.MaxSidebarTaskColorAutomationRules)
		for index := range rules {
			rules[index] = usermodels.SidebarTaskColorRule{
				ID: fmt.Sprintf("color-rule-%02d", index), Enabled: true,
				Condition: usermodels.SidebarTaskColorCondition{
					Dimension: usermodels.SidebarTaskColorDimensionTaskState,
					Value:     "FAILED", Label: "Failed",
				},
				Output: usermodels.SidebarTaskColorOutput{Kind: usermodels.SidebarTaskColorOutputFixed, Color: "blue"},
			}
		}
		automation, err := json.Marshal(usermodels.SidebarTaskColorAutomation{Enabled: true, Rules: rules})
		require.NoError(t, err)
		prefs.ColorSettings = &models.SidebarTaskColorSettings{ManualColors: manualColors, Automation: automation}
	}
	return query, prefs
}
