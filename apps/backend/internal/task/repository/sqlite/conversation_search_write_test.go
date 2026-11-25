package sqlite

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/conversationsearch"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestConversationSearchStreamingCost(t *testing.T) {
	if os.Getenv("KANDEV_SEARCH_SCALE") != "1" {
		t.Skip("opt-in streaming write measurement")
	}
	r := newConversationRepo(t)
	for i := 0; i < 10; i++ {
		sessionID, turnID := fmt.Sprintf("stream-session-%d", i), fmt.Sprintf("stream-turn-%d", i)
		seedForMsgTest(t, r, "stream-task", sessionID, turnID)
		require.NoError(t, r.CreateMessage(t.Context(), &models.Message{ID: fmt.Sprintf("stream-%d", i), TaskSessionID: sessionID, TurnID: turnID, AuthorType: models.MessageAuthorAgent, Content: "initial text"}))
	}
	for _, enabled := range []bool{false, true} {
		_, err := r.db.Exec(`DROP TRIGGER IF EXISTS task_conversation_message_update`)
		require.NoError(t, err)
		stop := func() {}
		if enabled {
			require.NoError(t, r.createConversationSearchSchema(t.Context()))
			stop = conversationsearch.Start(t.Context(), r, nil)
		}
		samples := make([]float64, 0, 1000)
		for tick := 0; tick < 100; tick++ {
			started := time.Now()
			for message := 0; message < 10; message++ {
				body := strings.Repeat("streamed output ", 64+tick*8)
				begin := time.Now()
				_, err = r.db.Exec(`UPDATE task_session_messages SET content=? WHERE id=?`, body, fmt.Sprintf("stream-%d", message))
				require.NoError(t, err)
				samples = append(samples, float64(time.Since(begin).Microseconds())/1000)
			}
			time.Sleep(max(0, 100*time.Millisecond-time.Since(started)))
		}
		stop()
		sort.Float64s(samples)
		t.Logf("indexing=%v write_p50_ms=%.3f write_p95_ms=%.3f", enabled, samples[500], samples[950])
	}
	_, err := r.db.Exec(`UPDATE task_session_messages SET content=? WHERE id='stream-0'`, strings.Repeat("large message ", 750000))
	require.NoError(t, err)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	_, err = r.ProcessConversationSearchBatch(t.Context(), false)
	require.NoError(t, err)
	runtime.ReadMemStats(&after)
	t.Logf("oversized_body_bytes=%d worker_elapsed=%s go_allocated_bytes=%d", len("large message ")*750000, time.Since(start), after.TotalAlloc-before.TotalAlloc)
}
