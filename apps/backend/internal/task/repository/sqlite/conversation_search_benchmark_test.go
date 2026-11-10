package sqlite

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// Opt-in scale evidence uses generated text only and reports every query class.
func TestConversationSearchPerformanceBudget(t *testing.T) {
	if os.Getenv("KANDEV_SEARCH_SCALE") != "1" {
		t.Skip("set KANDEV_SEARCH_SCALE=1 for the generated 100k/1m corpus")
	}
	measureConversationSearch(t, newConversationRepo(t))
}
func TestConversationSearchPostgresPerformanceBudget(t *testing.T) {
	if os.Getenv("KANDEV_SEARCH_SCALE") != "1" {
		t.Skip("set KANDEV_SEARCH_SCALE=1 for the generated 100k/1m corpus")
	}
	measureConversationSearch(t, openPostgresRepo(t))
}

func measureConversationSearch(t *testing.T, r *Repository) {
	t.Helper()
	started := time.Now()
	tx, err := r.db.Beginx()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	for i := 0; i < 10; i++ {
		_, err = tx.Exec(r.db.Rebind(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES(?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`), fmt.Sprintf("scale-w%d", i), "Scale")
		require.NoError(t, err)
	}
	for i := 0; i < 10000; i++ {
		id := fmt.Sprintf("scale-t%05d", i)
		_, err = tx.Exec(r.db.Rebind(`INSERT INTO tasks(id,workspace_id,title,description,state,priority,position,created_at,updated_at) VALUES(?,?,?,'','TODO','medium',0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`), id, fmt.Sprintf("scale-w%d", i%10), id)
		require.NoError(t, err)
		_, err = tx.Exec(r.db.Rebind(`INSERT INTO task_sessions(id,task_id,state,started_at,updated_at) VALUES(?,?,'COMPLETED',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`), id, id)
		require.NoError(t, err)
		_, err = tx.Exec(r.db.Rebind(`INSERT INTO task_session_turns(id,task_session_id,task_id,started_at,created_at,updated_at) VALUES(?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`), id, id, id)
		require.NoError(t, err)
	}
	_, err = tx.Exec(`UPDATE tasks SET archived_at=CURRENT_TIMESTAMP WHERE id LIKE 'scale-t09%'`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	prior := 0
	for _, size := range []int{100000, 1000000} {
		tx, err = r.db.Beginx()
		require.NoError(t, err)
		stmt, err := tx.Prepare(r.db.Rebind(`INSERT INTO task_session_messages(id,task_session_id,turn_id,task_id,author_type,type,content,created_at) VALUES(?,?,?,'','agent','message',?,CURRENT_TIMESTAMP)`))
		require.NoError(t, err)
		for i := prior; i < size; i++ {
			body := fmt.Sprintf("Common phrase %08d %s", i, strings.Repeat("ordinary generated conversation text. ", 27))
			if i%10000 == 0 {
				body += " selective-needle café 中文测试"
			}
			if i%10000 == 9900 {
				body += " archive-needle"
			}
			_, err = stmt.Exec(fmt.Sprintf("scale-m%08d", i), fmt.Sprintf("scale-t%05d", i%10000), fmt.Sprintf("scale-t%05d", i%10000), body)
			require.NoError(t, err)
		}
		require.NoError(t, stmt.Close())
		require.NoError(t, tx.Commit())
		build := time.Now()
		projection := `INSERT INTO task_conversation_documents(message_id,session_id,body,search_text,token,created_at)
   SELECT m.id,m.task_session_id,m.content,lower(m.content),q.token,m.created_at FROM task_session_messages m JOIN task_conversation_dirty q ON q.message_id=m.id`
		if dialect.IsPostgres(r.db.DriverName()) {
			projection = `INSERT INTO task_conversation_documents(message_id,session_id,body,search_text,token,created_at,index_text)
   SELECT m.id,m.task_session_id,m.content,lower(m.content),q.token,m.created_at,replace(replace(lower(m.content),'é','uc3ua9'),'中文测试','ue4ub8uadue6u96u87ue6ub5u8bue8uafu95') FROM task_session_messages m JOIN task_conversation_dirty q ON q.message_id=m.id`
		}
		_, err = r.db.Exec(projection)
		require.NoError(t, err)
		_, err = r.db.Exec(`DELETE FROM task_conversation_dirty`)
		require.NoError(t, err)
		_, err = r.db.Exec(`UPDATE task_conversation_index_state SET complete=1`)
		require.NoError(t, err)
		_, err = r.db.Exec(`ANALYZE`)
		require.NoError(t, err)
		logConversationSearchPlan(t, r)
		t.Logf("size=%d fixture_elapsed=%s index_build=%s", size, time.Since(started), time.Since(build))
		for _, query := range []string{"common phrase", "absentquartzword", "selective-needle", "café", "中文测试", "archive-needle"} {
			measureConversationQuery(t, r, size, query)
		}
		if dialect.IsPostgres(r.db.DriverName()) {
			var bytes int64
			require.NoError(t, r.db.Get(&bytes, `SELECT pg_total_relation_size('task_conversation_documents')`))
			t.Logf("projection_and_index_bytes=%d", bytes)
		} else {
			var bytes int64
			var pages int64
			require.NoError(t, r.db.Get(&pages, `PRAGMA page_count`))
			require.NoError(t, r.db.Get(&bytes, `PRAGMA page_size`))
			t.Logf("whole_database_allocated_bytes=%d", pages*bytes)
		}
		prior = size
	}
}

func measureConversationQuery(t *testing.T, r *Repository, size int, query string) {
	t.Helper()
	samples := make([]float64, 0, 30)
	successful := make([]float64, 0, 30)
	states := make(map[string]int)
	errors, limited, unexpected, more := 0, 0, 0, 0
	for i := 0; i < 33; i++ {
		ctx, cancel := context.WithTimeout(t.Context(), 750*time.Millisecond)
		start := time.Now()
		page, err := r.SearchTaskConversations(ctx, "scale-w0", query, 20)
		elapsed := float64(time.Since(start).Microseconds()) / 1000
		cancel()
		if i < 3 {
			continue
		}
		samples = append(samples, elapsed)
		if err != nil {
			errors++
			continue
		}
		if page == nil {
			unexpected++
			continue
		}
		states[page.State]++
		if page.HasMore {
			more++
		}
		if page.State == models.ConversationSearchLimited {
			limited++
			continue
		}
		if page.State != models.ConversationSearchReady {
			continue
		}
		if !conversationMeasurementMatches(query, page) {
			unexpected++
			continue
		}
		successful = append(successful, elapsed)
	}
	p50, p95 := conversationMeasuredLatency(samples)
	success50, success95 := conversationMeasuredLatency(successful)
	t.Logf("size=%d query=%q all_p50_ms=%.2f all_p95_ms=%.2f success_p50_ms=%.2f success_p95_ms=%.2f successful=%d limited=%d errors=%d unexpected=%d has_more=%d states=%v", size, query, p50, p95, success50, success95, len(successful), limited, errors, unexpected, more, states)
	if unexpected > 0 {
		t.Errorf("query %q returned %d unexpected result pages", query, unexpected)
	}
}

func conversationMeasurementMatches(query string, page *models.ConversationSearchPage) bool {
	if page == nil || page.State != models.ConversationSearchReady {
		return false
	}
	expectedTask := ""
	switch query {
	case "absentquartzword":
		return len(page.Hits) == 0
	case "selective-needle", "café", "中文测试":
		expectedTask = "scale-t00000"
	case "archive-needle":
		expectedTask = "scale-t09900"
	case "common phrase":
	default:
		return false
	}
	if len(page.Hits) == 0 {
		return false
	}
	for _, hit := range page.Hits {
		if !strings.Contains(strings.ToLower(hit.Match.Excerpt), query) {
			return false
		}
		if expectedTask != "" && hit.TaskID != expectedTask {
			return false
		}
	}
	return true
}

func conversationMeasuredLatency(samples []float64) (float64, float64) {
	if len(samples) == 0 {
		return math.NaN(), math.NaN()
	}
	sort.Float64s(samples)
	return samples[len(samples)/2], samples[int(math.Ceil(float64(len(samples))*0.95))-1]
}

func logConversationSearchPlan(t *testing.T, r *Repository) {
	t.Helper()
	statement, pattern := conversationSearchSQL(r.db.DriverName(), "selective-needle")
	prefix := "EXPLAIN QUERY PLAN "
	if dialect.IsPostgres(r.db.DriverName()) {
		prefix = "EXPLAIN "
	}
	rows, err := r.ro.Query(r.ro.Rebind(prefix+statement), "selective-needle", pattern, "selective-needle", "scale-w0", "selective-needle", pattern, "selective-needle", "scale-w0")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var detail string
		if dialect.IsPostgres(r.db.DriverName()) {
			require.NoError(t, rows.Scan(&detail))
		} else {
			var id, parent, unused int
			require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		}
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	t.Logf("selective_query_plan:\n%s", strings.Join(plan, "\n"))
}

func TestConversationSearchMeasurementOutcomes(t *testing.T) {
	correct := &models.ConversationSearchPage{State: models.ConversationSearchReady, Hits: []models.ConversationSearchHit{{TaskID: "scale-t00000", Match: models.ConversationMatch{Excerpt: "selective-needle café 中文测试"}}}}
	require.True(t, conversationMeasurementMatches("selective-needle", correct))
	require.True(t, conversationMeasurementMatches("café", correct))
	require.True(t, conversationMeasurementMatches("中文测试", correct))
	require.False(t, conversationMeasurementMatches("archive-needle", correct))
	require.False(t, conversationMeasurementMatches("absentquartzword", correct))
	require.True(t, conversationMeasurementMatches("absentquartzword", &models.ConversationSearchPage{State: models.ConversationSearchReady}))
	require.False(t, conversationMeasurementMatches("selective-needle", &models.ConversationSearchPage{State: models.ConversationSearchReady}))
	correct.State = models.ConversationSearchLimited
	require.False(t, conversationMeasurementMatches("selective-needle", correct))
	require.False(t, conversationMeasurementMatches("selective-needle", nil))
}

func TestConversationSearchMeasuredLatency(t *testing.T) {
	p50, p95 := conversationMeasuredLatency(nil)
	require.True(t, math.IsNaN(p50))
	require.True(t, math.IsNaN(p95))
	p50, p95 = conversationMeasuredLatency([]float64{3})
	require.Equal(t, float64(3), p50)
	require.Equal(t, float64(3), p95)
}
