package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// insertPromptRow inserts a raw message row with a driver-serialized timestamp,
// bypassing the create boundary — the legacy-row shape the indexed readers
// must still order correctly. authorType must be "user" or "agent".
func insertPromptRow(t *testing.T, repo *Repository, id, sessionID, turnID, authorType, content string, ts time.Time) {
	t.Helper()
	insertAgentMsg(t, repo, id, sessionID, turnID, authorType, content, ts)
}

// insertPromptRowWithRawTimestamp inserts a message row whose created_at is a
// raw TEXT value (e.g. carrying a non-UTC offset), bypassing the driver's
// time.Time serialization — the legacy/harness-seed shape whose offset the
// normalized-microsecond helper must convert to UTC.
func insertPromptRowWithRawTimestamp(t *testing.T, repo *Repository, id, sessionID, turnID, authorType, content, rawCreatedAt string) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, '', ?, ?, '', ?, 0, 'message', '{}', ?)
	`), id, sessionID, turnID, authorType, content, rawCreatedAt)
	if err != nil {
		t.Fatalf("insert message %s: %v", id, err)
	}
}

// backfillPromptSeqForTest assigns fixture-inserted legacy rows their derived
// ordinals exactly as the production migration does (see backfillPromptSeq),
// so the fixture tests exercise the same upgrade path the indexed readers
// serve after migration.
func backfillPromptSeqForTest(t *testing.T, repo *Repository) {
	t.Helper()
	if err := repo.backfillPromptSeq(); err != nil {
		t.Fatalf("backfill prompt_seq: %v", err)
	}
}

// assertPromptOrdinals asserts the indexed read (persisted prompt_seq) and
// the paginated read agree on every user row's ordinal and that both return
// zero for agent rows.
func assertPromptOrdinals(t *testing.T, ctx context.Context, repo *Repository, sessionID string, want map[string]int) {
	t.Helper()
	for id, wantIndex := range want {
		got, err := repo.GetMessageWithPromptIndex(ctx, id)
		if err != nil {
			t.Fatalf("GetMessageWithPromptIndex(%s): %v", id, err)
		}
		if got.PromptIndex != wantIndex {
			t.Errorf("GetMessageWithPromptIndex(%s).PromptIndex = %d, want %d", id, got.PromptIndex, wantIndex)
		}
	}
	// Full-session window read: every page in ordinal order must agree.
	pages := paginateAll(t, ctx, repo, sessionID, 2, "")
	byID := map[string]int{}
	for _, page := range pages {
		for _, m := range page {
			byID[m.ID] = m.PromptIndex
		}
	}
	for id, wantIndex := range want {
		if byID[id] != wantIndex {
			t.Errorf("ListMessagesPaginated(%s).PromptIndex = %d, want %d", id, byID[id], wantIndex)
		}
	}
}

// paginateAll walks the session from the newest page backward in pages of
// `limit`, returning every loaded page. stopBeforeID, when non-empty, stops
// once a page contains it (inclusive) — used to assert the page that carries
// a lower-ordinal prompt never skips a higher-ordinal one.
func paginateAll(t *testing.T, ctx context.Context, repo *Repository, sessionID string, limit int, stopBeforeID string) [][]*models.Message {
	t.Helper()
	var pages [][]*models.Message
	cursor := ""
	for {
		opts := models.ListMessagesOptions{Limit: limit, Sort: "desc"}
		if cursor != "" {
			opts.Before = cursor
		}
		msgs, hasMore, err := repo.ListMessagesPaginated(ctx, sessionID, opts)
		if err != nil {
			t.Fatalf("ListMessagesPaginated: %v", err)
		}
		pages = append(pages, msgs)
		if len(msgs) > 0 && stopBeforeID != "" {
			for _, m := range msgs {
				if m.ID == stopBeforeID {
					return pages
				}
			}
		}
		if !hasMore || len(msgs) == 0 {
			return pages
		}
		cursor = msgs[len(msgs)-1].ID
	}
}

// frontendMicrosecondKey reproduces the frontend's `epochNanoseconds / 1000n`
// floor division for a parsed RFC3339 timestamp, formatted to the exact
// normalized key bytes the backend produces.
func frontendMicrosecondKey(t *testing.T, rfc3339 string) string {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, rfc3339)
	if err != nil {
		t.Fatalf("parse %q: %v", rfc3339, err)
	}
	return formatPromptKey(time.Unix(0, (parsed.UnixNano()/1000)*1000))
}

// TestPromptIndexReadsAndPagination verifies derived prompt ordinals across live creation, indexed reads, and paginated lists.
func TestPromptIndexReadsAndPagination(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Two sessions with interleaved user/agent rows, including two user
	// messages sharing an identical created_at (legacy raw rows; the create
	// boundary only applies to rows inserted through CreateMessage).
	seedForMsgTest(t, repo, "task-PA", "sess-PA", "turn-PA")
	insertPromptRow(t, repo, "pa-agent-1", "sess-PA", "turn-PA", "agent", "reply 1", now)
	insertPromptRow(t, repo, "pa-user-1", "sess-PA", "turn-PA", "user", "prompt 1", now)
	insertPromptRow(t, repo, "pa-user-2", "sess-PA", "turn-PA", "user", "prompt 2", now.Add(time.Second))
	insertPromptRow(t, repo, "pa-agent-2", "sess-PA", "turn-PA", "agent", "reply 2", now.Add(2*time.Second))
	insertPromptRow(t, repo, "pa-user-3", "sess-PA", "turn-PA", "user", "prompt 3", now.Add(2*time.Second))
	insertPromptRow(t, repo, "pa-user-tie", "sess-PA", "turn-PA", "user", "prompt tie", now)

	seedForMsgTest(t, repo, "task-PB", "sess-PB", "turn-PB")
	insertPromptRow(t, repo, "pb-user-1", "sess-PB", "turn-PB", "user", "prompt 1", now.Add(time.Minute))
	insertPromptRow(t, repo, "pb-user-2", "sess-PB", "turn-PB", "user", "prompt 2", now.Add(2*time.Minute))

	// The fixture rows bypass the create boundary (the legacy-row shape), so
	// assign ordinals with the production backfill like a migrated database.
	backfillPromptSeqForTest(t, repo)

	// Ordinary hot paths stay legacy: GetMessage and full ListMessages return
	// zero PromptIndex even for user rows.
	legacy, err := repo.GetMessage(ctx, "pa-user-1")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if legacy.PromptIndex != 0 {
		t.Errorf("GetMessage(user).PromptIndex = %d, want 0 (legacy hot path)", legacy.PromptIndex)
	}
	full, err := repo.ListMessages(ctx, "sess-PA")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for _, m := range full {
		if m.PromptIndex != 0 {
			t.Errorf("ListMessages(%s).PromptIndex = %d, want 0 (legacy hot path)", m.ID, m.PromptIndex)
		}
	}

	// User ordinals: (created_at, id) ascending, ties broken by id. The two
	// identical-timestamp user rows (pa-user-1 and pa-user-tie) order by id.
	wantA := map[string]int{
		"pa-user-1":   1,
		"pa-user-tie": 2, // same microsecond as pa-user-1, later id
		"pa-user-2":   3,
		"pa-user-3":   4,
		"pa-agent-1":  0,
		"pa-agent-2":  0,
	}
	assertPromptOrdinals(t, ctx, repo, "sess-PA", wantA)

	// Cross-session isolation: session B's numbering restarts at 1.
	wantB := map[string]int{"pb-user-1": 1, "pb-user-2": 2}
	assertPromptOrdinals(t, ctx, repo, "sess-PB", wantB)
}

// TestPromptIndexTiedMicrosecondPagesContiguous verifies that pages split within a tied-microsecond burst keep contiguous ordinals.
func TestPromptIndexTiedMicrosecondPagesContiguous(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-TIE", "sess-TIE", "turn-TIE")

	// Nanosecond-distinct raw timestamps sharing one microsecond, with
	// REVERSED id order: the later instant has the smaller id. Ordering must
	// be by (microsecond, id), so the smaller id comes first.
	tsLater := time.Date(2026, 8, 18, 10, 0, 0, 123456789, time.UTC)
	tsEarlier := time.Date(2026, 8, 18, 10, 0, 0, 123456001, time.UTC)
	insertPromptRow(t, repo, "tie-zzz", "sess-TIE", "turn-TIE", "user", "later instant, later id", tsLater)
	insertPromptRow(t, repo, "tie-aaa", "sess-TIE", "turn-TIE", "user", "earlier instant, earlier id", tsEarlier)
	insertPromptRow(t, repo, "tie-mmm", "sess-TIE", "turn-TIE", "user", "middle id, same microsecond", tsLater)

	backfillPromptSeqForTest(t, repo)

	want := map[string]int{"tie-aaa": 1, "tie-mmm": 2, "tie-zzz": 3}
	assertPromptOrdinals(t, ctx, repo, "sess-TIE", want)

	// The SQLite normalized key equals the frontend epochNanoseconds/1000n
	// floor division for the same rows.
	nm := dialect.NormalizedMicrosecond(dialect.SQLite3, "created_at")
	var got string
	if err := repo.ro.QueryRowContext(ctx, repo.ro.Rebind("SELECT "+nm+" FROM task_session_messages WHERE id = ?"), "tie-zzz").Scan(&got); err != nil {
		t.Fatalf("query normalized key: %v", err)
	}
	if wantKey := frontendMicrosecondKey(t, "2026-08-18T10:00:00.123456789Z"); got != wantKey {
		t.Errorf("normalized key = %q, want frontend floor key %q", got, wantKey)
	}

	// Paginate backward with a small page; the page containing prompt #1 must
	// never skip a higher-ordinal prompt. All three prompts must appear.
	pages := paginateAll(t, ctx, repo, "sess-TIE", 2, "tie-aaa")
	var seen []string
	for _, page := range pages {
		for _, m := range page {
			seen = append(seen, m.ID)
		}
	}
	wantOrder := []string{"tie-zzz", "tie-mmm", "tie-aaa"}
	if strings.Join(seen, ",") != strings.Join(wantOrder, ",") {
		t.Errorf("pagination order = %v, want %v (contiguous, no dup/skip)", seen, wantOrder)
	}

	// Non-first pages return absolute (session-wide) prompt_index values.
	var lastPageOrdinals []int
	for _, m := range pages[len(pages)-1] {
		lastPageOrdinals = append(lastPageOrdinals, m.PromptIndex)
	}
	if fmt.Sprint(lastPageOrdinals) != "[1]" {
		t.Errorf("oldest page ordinals = %v, want absolute [1]", lastPageOrdinals)
	}
}

// TestPromptIndexOffsetRowsMatchFrontendUTCKey verifies the offset-style key matches the frontend UTC normalized key for the same rows.
func TestPromptIndexOffsetRowsMatchFrontendUTCKey(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-OFF", "sess-OFF", "turn-OFF")

	// A seed row carrying a +02:00 offset: its UTC instant is 08:00:00, and
	// the frontend's UTC key must match the backend's normalized key.
	insertPromptRowWithRawTimestamp(t, repo, "off-user-1", "sess-OFF", "turn-OFF", "user", "offset prompt 1", "2026-08-18 10:00:00.123456+02:00")
	insertPromptRowWithRawTimestamp(t, repo, "off-user-2", "sess-OFF", "turn-OFF", "user", "offset prompt 2", "2026-08-18 10:00:01.000000+02:00")
	insertPromptRowWithRawTimestamp(t, repo, "off-agent", "sess-OFF", "turn-OFF", "agent", "offset reply", "2026-08-18 10:00:02.000000+02:00")

	backfillPromptSeqForTest(t, repo)

	want := map[string]int{"off-user-1": 1, "off-user-2": 2, "off-agent": 0}
	assertPromptOrdinals(t, ctx, repo, "sess-OFF", want)

	// The SQL normalized key equals the frontend UTC floor key for the row.
	nm := dialect.NormalizedMicrosecond(dialect.SQLite3, "created_at")
	var got string
	if err := repo.ro.QueryRowContext(ctx, repo.ro.Rebind("SELECT "+nm+" FROM task_session_messages WHERE id = ?"), "off-user-1").Scan(&got); err != nil {
		t.Fatalf("query normalized key: %v", err)
	}
	if wantKey := frontendMicrosecondKey(t, "2026-08-18T10:00:00.123456+02:00"); got != wantKey {
		t.Errorf("normalized key = %q, want frontend UTC key %q", got, wantKey)
	}

	// Paginate with the offset-carrying row as the cursor: pages must be
	// contiguous in ordinal order (normalized string bound, not the raw
	// offset string), with no duplicated or skipped rows. The agent row at
	// 10:00:02+02:00 (08:00:02 UTC) is the newest row overall.
	order := []string{}
	cursor := ""
	for {
		msgs, hasMore, err := repo.ListMessagesPaginated(ctx, "sess-OFF", models.ListMessagesOptions{Limit: 1, Sort: "desc", Before: cursor})
		if err != nil {
			t.Fatalf("paginate: %v", err)
		}
		if len(msgs) == 1 {
			order = append(order, msgs[0].ID)
		}
		if !hasMore || len(msgs) == 0 {
			break
		}
		cursor = msgs[0].ID
	}
	wantOrder := []string{"off-agent", "off-user-2", "off-user-1"}
	if strings.Join(order, ",") != strings.Join(wantOrder, ",") {
		t.Errorf("pagination order = %v, want %v", order, wantOrder)
	}

	// The cursor-key derivation SELECT returns the UTC-normalized key, never a
	// driver-parsed RFC3339Nano serialization.
	cursorKey, err := repo.messageCursorKey(ctx, "off-user-1")
	if err != nil {
		t.Fatalf("messageCursorKey: %v", err)
	}
	if cursorKey != frontendMicrosecondKey(t, "2026-08-18T10:00:00.123456+02:00") {
		t.Errorf("cursor key = %q, want normalized UTC key %q", cursorKey, frontendMicrosecondKey(t, "2026-08-18T10:00:00.123456+02:00"))
	}
	if strings.Contains(cursorKey, "T") || strings.HasSuffix(cursorKey, "Z") {
		t.Errorf("cursor key %q must be the space-separated UTC form, never RFC3339Nano", cursorKey)
	}
}
