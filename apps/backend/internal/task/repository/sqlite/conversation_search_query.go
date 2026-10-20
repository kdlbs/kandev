package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func conversationQuery(query string) (string, string) {
	query = strings.ToLower(strings.TrimSpace(query))
	if strings.ContainsRune(query, 0) {
		return query, models.ConversationQueryTooBroad
	}
	n := utf8.RuneCountInString(query)
	if n < 3 {
		return query, models.ConversationQueryTooShort
	}
	if n > 200 {
		return query, models.ConversationQueryTooLong
	}
	run := 0
	for _, c := range query {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			run++
		} else {
			run = 0
		}
		if run >= 3 {
			return query, models.ConversationSearchReady
		}
	}
	return query, models.ConversationQueryTooBroad
}

func (r *Repository) SearchTaskConversations(ctx context.Context, workspaceID, query string, limit int) (*models.ConversationSearchPage, error) {
	query, state := conversationQuery(query)
	page := &models.ConversationSearchPage{Hits: []models.ConversationSearchHit{}, State: state}
	if state != models.ConversationSearchReady {
		return page, nil
	}
	if !r.ConversationSearchReady(ctx) {
		page.State = models.ConversationSearchUnavailable
		return page, nil
	}
	limit = min(max(limit, 1), 20)
	statement, pattern := conversationSearchSQL(r.ro.DriverName(), query)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(statement), query, pattern, query, workspaceID, query, pattern, query, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	scanned, err := collectConversationHits(rows, page, query, limit)
	if err != nil {
		return nil, err
	}
	pending, err := r.ConversationSearchPending(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if pending {
		page.State = models.ConversationSearchIndexing
	}
	if scanned > 200 && len(page.Hits) < limit {
		page.State = models.ConversationSearchLimited
	}
	return page, nil
}

func collectConversationHits(rows *sql.Rows, page *models.ConversationSearchPage, query string, limit int) (int, error) {
	seen := make(map[string]bool)
	scanned := 0
	for rows.Next() {
		var hit models.ConversationSearchHit
		err := rows.Scan(&hit.TaskID, &hit.Match.SessionID, &hit.Match.MessageID, &hit.Match.AuthorType, &hit.Match.Excerpt)
		if err != nil {
			return scanned, err
		}
		scanned++
		if scanned > 200 {
			page.HasMore = true
			break
		}
		if seen[hit.TaskID] {
			continue
		}
		if len(page.Hits) == limit {
			page.HasMore = true
			break
		}
		hit.Match.HighlightRanges = conversationHighlight(hit.Match.Excerpt, query)
		if len(hit.Match.HighlightRanges) == 0 {
			continue
		}
		seen[hit.TaskID] = true
		page.Hits = append(page.Hits, hit)
	}
	if err := rows.Err(); err != nil {
		return scanned, err
	}
	if err := rows.Close(); err != nil {
		return scanned, err
	}
	return scanned, nil
}

func conversationSearchSQL(driver, query string) (string, string) {
	from := `task_conversation_fts JOIN task_conversation_documents d ON d.document_id=task_conversation_fts.rowid`
	predicate := `task_conversation_fts MATCH ?`
	position := `max(instr(d.search_text,?)-20,1)`
	exact := `instr(d.search_text,?)>0`
	pattern := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	if dialect.IsPostgres(driver) {
		from = `task_conversation_documents d`
		predicate = `d.index_text LIKE ? ESCAPE '\'`
		exact = `strpos(d.search_text,?)>0`
		position = `greatest(strpos(d.search_text,?)-20,1)`
		pattern = "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(conversationIndexKey(query)) + "%"
	}
	// Bound visible candidates before sorting text matches. Active candidates take
	// the budget first; only this bounded window is ranked by message recency.
	candidate := fmt.Sprintf(`SELECT t.id AS task_id,s.id AS session_id,m.id AS message_id,m.author_type,substr(d.body,%s,240) AS excerpt,
   t.archived_at,m.created_at
   FROM %s JOIN task_session_messages m ON m.id=d.message_id
   JOIN task_sessions s ON s.id=m.task_session_id JOIN tasks t ON t.id=s.task_id
   WHERE %s AND %s AND t.workspace_id=? AND t.is_ephemeral=0 %s
   AND NOT EXISTS(SELECT 1 FROM task_conversation_dirty q WHERE q.message_id=m.id)`, position, from, predicate, exact, andNotAutomationOriginT)
	return `WITH candidates AS MATERIALIZED (
  SELECT * FROM (` + candidate + ` AND t.archived_at IS NULL LIMIT 201) active_candidates
  UNION ALL
  SELECT * FROM (` + candidate + ` AND t.archived_at IS NOT NULL LIMIT 201) archived_candidates
  LIMIT 201)
 SELECT task_id,session_id,message_id,author_type,excerpt
 FROM candidates ORDER BY CASE WHEN archived_at IS NULL THEN 0 ELSE 1 END,created_at DESC,message_id DESC`, pattern

}

func conversationHighlight(excerpt, query string) [][2]int {
	text := strings.ToLower(excerpt)
	pos := strings.Index(text, query)
	if pos < 0 {
		return nil
	}
	start := utf8.RuneCountInString(text[:pos])
	return [][2]int{{start, start + utf8.RuneCountInString(query)}}
}
