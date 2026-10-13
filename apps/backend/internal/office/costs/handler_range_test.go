package costs_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/costs"
)

// @covers AC-OFFICE-COSTS-001.2 AC-OFFICE-COSTS-001.8
func TestCostsBreakdownRangeHTTP(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Advance only the bubble's clock to a mid-month instant.
		time.Sleep(12 * 24 * time.Hour)
		svc, execSQL := newTestCostService(t)
		now := time.Now().UTC()
		month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		for i, event := range []struct {
			at   time.Time
			cost int64
		}{
			{now.Add(-60 * 24 * time.Hour), 20000},
			{month.Add(-time.Hour), 30000},
			{now.Add(-time.Hour), 10000},
		} {
			key := fmt.Sprintf("range-%d", i)
			execSQL(`INSERT INTO tasks (id, workspace_id, project_id) VALUES (?, ?, ?)`, key, "range-ws", key)
			execSQL(`INSERT INTO agent_profiles (id, name) VALUES (?, ?)`, key, key)
			execSQL(`INSERT INTO office_projects (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, key, "range-ws", key, now, now)
			execSQL(`INSERT INTO office_cost_events (id, task_id, agent_profile_id, model, provider, cost_subcents, occurred_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, key, key, key, key, key, event.cost, event.at, now)
		}
		gin.SetMode(gin.TestMode)
		router := gin.New()
		costs.NewHandler(svc).RegisterRoutes(router.Group("/api/v1/office"))
		for _, tc := range []struct {
			query  string
			total  int64
			groups int
		}{
			{"", 60000, 3}, {"?range=mtd", 10000, 1}, {"?range=30d", 40000, 2},
		} {
			got := readRangeBreakdown(t, router, "range-ws"+tc.query)
			if got.TotalSubcents != tc.total {
				t.Errorf("total = %d, want %d", got.TotalSubcents, tc.total)
			}
			for name, items := range map[string][]*costs.CostBreakdown{"agent": got.ByAgent, "project": got.ByProject, "model": got.ByModel, "provider": got.ByProvider} {
				var sum int64
				var count int
				for _, item := range items {
					sum += item.TotalSubcents
					count += item.Count
				}
				if len(items) != tc.groups || sum != tc.total || count != tc.groups {
					t.Errorf("%s: groups=%d sum=%d count=%d, want %d/%d/%d", name, len(items), sum, count, tc.groups, tc.total, tc.groups)
				}
			}
		}
		empty := readRangeBreakdown(t, router, "empty-ws?range=mtd")
		if empty.TotalSubcents != 0 || empty.ByAgent == nil || empty.ByProject == nil || empty.ByModel == nil || empty.ByProvider == nil || len(empty.ByAgent)+len(empty.ByProject)+len(empty.ByModel)+len(empty.ByProvider) != 0 {
			t.Errorf("empty window = %+v, want zero and four empty arrays", empty)
		}
		for _, query := range []string{"?range=", "?range=year", "?range=MTD", "?range=mtd&range=30d", "?range=mtd&range=mtd"} {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/range-ws/costs/breakdown"+query, nil))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s status=%d, want 400", query, rec.Code)
			}
		}
	})
}

func readRangeBreakdown(t *testing.T, router *gin.Engine, workspaceQuery string) costs.CostBreakdownComposedResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/"+workspaceQueryPath(workspaceQuery), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d: %s", rec.Code, rec.Body.String())
	}
	var got costs.CostBreakdownComposedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func workspaceQueryPath(workspaceQuery string) string {
	for i, c := range workspaceQuery {
		if c == '?' {
			return workspaceQuery[:i] + "/costs/breakdown" + workspaceQuery[i:]
		}
	}
	return workspaceQuery + "/costs/breakdown"
}

func TestCostsBreakdownRangeStorageFailure(t *testing.T) {
	svc, execSQL := newTestCostService(t)
	execSQL(`DROP TABLE office_cost_events`)
	router := gin.New()
	costs.NewHandler(svc).RegisterRoutes(router.Group("/api/v1/office"))
	for _, tc := range []struct {
		query  string
		status int
	}{{"?range=mtd", http.StatusInternalServerError}, {"?range=invalid", http.StatusBadRequest}} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws/costs/breakdown"+tc.query, nil))
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if rec.Code != tc.status || body["error"] == "" {
			t.Errorf("%s status=%d body=%v", tc.query, rec.Code, body)
		}
	}
}
