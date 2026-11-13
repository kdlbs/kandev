package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// unreadWalkPageSize bounds each backwards page of the unread walk. A cursor
// whose message was deleted has no lower bound, so the walk must stay paged.
const unreadWalkPageSize = 64

// unreadTailParams is the most bind parameters one session adds to a tail
// query: session id, cursor key (twice), cursor id, and the page limit.
const unreadTailParams = 5

// sessionCursor is a session's read cursor. key is the cursor message's
// normalized created_at exactly as the database returns it; nil means the
// session has no cursor or its cursor message no longer exists, so its walk
// has no lower bound. The database-native key is bound back into the walk
// predicates, so Go never formats a timestamp for SQL.
type sessionCursor struct {
	rank statussummary.UnreadRank
	key  interface{}
}

// unreadPosition locates a message in the walk ordering. A zero id means no
// bound on that side.
type unreadPosition struct {
	key interface{}
	id  string
}

// ListSessionUnreadState returns the read cursor rank and newest
// unread-qualifying rank for every requested session that still exists. A
// session without a cursor has a zero read rank. Ordering is the
// microsecond-normalized (created_at, id) key that UnreadRank compares and
// UpdateTaskSessionLastReadMessageID guards, so the SQL walk and the
// in-process rank agree.
//
// Every session resolves in one LIMIT-bounded tail query per parameter chunk,
// bounded below by its cursor when it has a live one. Only a session whose
// tail fills a full page without a qualifying row falls back to the paged
// walk, seeded past that tail.
func (r *Repository) ListSessionUnreadState(
	ctx context.Context,
	sessionIDs []string,
) (map[string]statussummary.SessionUnreadState, error) {
	result := make(map[string]statussummary.SessionUnreadState, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return result, nil
	}
	cursors, err := r.loadSessionReadCursors(ctx, sessionIDs)
	if err != nil {
		return nil, err
	}
	existing := make([]string, 0, len(cursors))
	for sessionID := range cursors {
		existing = append(existing, sessionID)
	}
	sort.Strings(existing)
	tails, err := r.loadUnreadTails(ctx, cursors, existing)
	if err != nil {
		return nil, err
	}
	for _, sessionID := range existing {
		cursor := cursors[sessionID]
		tail := tails[sessionID]
		newest := cursor.rank
		found := false
		for _, row := range tail {
			if row.qualifies {
				newest = row.rank
				found = true
				break
			}
		}
		if !found && len(tail) == unreadWalkPageSize {
			last := tail[len(tail)-1]
			newest, err = r.newestUnreadQualifyingRank(ctx, sessionID, cursor, unreadPosition{key: last.key, id: last.rank.ID})
			if err != nil {
				return nil, err
			}
		}
		result[sessionID] = statussummary.SessionUnreadState{Read: cursor.rank, Newest: newest}
	}
	return result, nil
}

// loadSessionReadCursors returns the cursor of every requested session that
// exists. A session without a cursor has a zero rank; a cursor whose message
// was deleted keeps its id with no time. Neither has a key.
func (r *Repository) loadSessionReadCursors(
	ctx context.Context,
	sessionIDs []string,
) (map[string]sessionCursor, error) {
	cursors := make(map[string]sessionCursor, len(sessionIDs))
	nmCur := dialect.NormalizedMicrosecond(r.ro.DriverName(), "m.created_at")
	for _, chunk := range chunkIDs(sessionIDs, sqliteMaxHostParams) {
		placeholders, ids := buildInPlaceholders(chunk)
		query := `
			SELECT s.id, COALESCE(s.last_read_message_id, ''), m.created_at, ` + nmCur + `
			FROM task_sessions s
			LEFT JOIN task_session_messages m ON m.id = s.last_read_message_id
			WHERE s.id IN (` + placeholders + `)`
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), ids...)
		if err != nil {
			return nil, fmt.Errorf("load session read cursors: %w", err)
		}
		for rows.Next() {
			var sessionID, cursorID string
			var rawCreatedAt, key interface{}
			if err := rows.Scan(&sessionID, &cursorID, &rawCreatedAt, &key); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan session read cursor: %w", err)
			}
			cursor := sessionCursor{rank: statussummary.UnreadRank{ID: cursorID}}
			if rawCreatedAt != nil {
				createdAt, err := parseTaskActivityTime(rawCreatedAt)
				if err != nil {
					_ = rows.Close()
					return nil, fmt.Errorf("parse read cursor time for session %q: %w", sessionID, err)
				}
				cursor.rank = statussummary.NewUnreadRank(cursorID, createdAt)
				cursor.key = key
			}
			cursors[sessionID] = cursor
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("read session read cursors: %w", err)
		}
		_ = rows.Close()
	}
	return cursors, nil
}

type unreadWalkRow struct {
	rank      statussummary.UnreadRank
	key       interface{}
	qualifies bool
}

// loadUnreadTails returns, per session, up to one page of rows newest first,
// bounded below by the session's cursor when it has a live key. Each session
// is its own LIMIT-bounded subquery whose cursor bound is a bind parameter on
// the prompt-order index expression, so the database seeks the tail instead
// of scanning the whole session.
func (r *Repository) loadUnreadTails(
	ctx context.Context,
	cursors map[string]sessionCursor,
	sessionIDs []string,
) (map[string][]unreadWalkRow, error) {
	tails := make(map[string][]unreadWalkRow, len(sessionIDs))
	nmMsg := dialect.NormalizedMicrosecond(r.ro.DriverName(), "m.created_at")
	for _, chunk := range chunkIDs(sessionIDs, sqliteMaxHostParams/unreadTailParams) {
		parts := make([]string, 0, len(chunk))
		args := make([]interface{}, 0, unreadTailParams*len(chunk))
		for _, sessionID := range chunk {
			cursor := cursors[sessionID]
			bound := ""
			args = append(args, sessionID)
			if cursor.key != nil {
				bound = `
				  AND ` + nmMsg + ` >= ?
				  AND (` + nmMsg + ` > ? OR m.id > ?)`
				args = append(args, cursor.key, cursor.key, cursor.rank.ID)
			}
			parts = append(parts, `SELECT * FROM (
				SELECT m.task_session_id, m.id, m.author_type, m.type,
				       CASE WHEN m.type = 'status' THEN m.content ELSE '' END AS content,
				       m.metadata, m.created_at, `+nmMsg+` AS msg_key
				FROM task_session_messages m
				WHERE m.task_session_id = ?`+unreadCandidateFilter+bound+`
				ORDER BY `+nmMsg+` DESC, m.id DESC
				LIMIT ?
			) t`)
			args = append(args, unreadWalkPageSize)
		}
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(strings.Join(parts, " UNION ALL ")), args...)
		if err != nil {
			return nil, fmt.Errorf("load unread tails: %w", err)
		}
		for rows.Next() {
			var sessionID, id, authorType, messageType, content, metadataJSON string
			var rawCreatedAt, key interface{}
			if err := rows.Scan(&sessionID, &id, &authorType, &messageType, &content, &metadataJSON, &rawCreatedAt, &key); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan unread tail row: %w", err)
			}
			row, err := buildUnreadWalkRow(id, authorType, messageType, content, metadataJSON, rawCreatedAt, key)
			if err != nil {
				_ = rows.Close()
				return nil, err
			}
			tails[sessionID] = append(tails[sessionID], row)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("read unread tails: %w", err)
		}
		_ = rows.Close()
	}
	for sessionID := range tails {
		tail := tails[sessionID]
		sort.SliceStable(tail, func(i, j int) bool { return tail[i].rank.After(tail[j].rank) })
	}
	return tails, nil
}

// newestUnreadQualifyingRank walks the session backwards in bounded pages,
// stopping at the first qualifying message or at the cursor. boundary is the
// oldest message already scanned, or the zero position to start from the
// newest. A missing or deleted cursor (nil key) has no lower bound. It returns
// the cursor rank when nothing after the cursor qualifies.
func (r *Repository) newestUnreadQualifyingRank(
	ctx context.Context,
	sessionID string,
	cursor sessionCursor,
	boundary unreadPosition,
) (statussummary.UnreadRank, error) {
	for {
		page, err := r.unreadWalkPage(ctx, sessionID, cursor, boundary)
		if err != nil {
			return statussummary.UnreadRank{}, err
		}
		for _, row := range page {
			if row.qualifies {
				return row.rank, nil
			}
		}
		if len(page) < unreadWalkPageSize {
			return cursor.rank, nil
		}
		last := page[len(page)-1]
		boundary = unreadPosition{key: last.key, id: last.rank.ID}
	}
}

// unreadWalkPage loads one page of messages after the cursor and before the
// boundary, newest first. Each bound is a bind parameter on the normalized
// created_at expression, so the prompt-order index range-seeks it. Only status
// rows need their content.
func (r *Repository) unreadWalkPage(
	ctx context.Context,
	sessionID string,
	cursor sessionCursor,
	boundary unreadPosition,
) ([]unreadWalkRow, error) {
	nmMsg := dialect.NormalizedMicrosecond(r.ro.DriverName(), "m.created_at")
	query := `
		SELECT m.id, m.author_type, m.type, CASE WHEN m.type = 'status' THEN m.content ELSE '' END, m.metadata, m.created_at, ` + nmMsg + `
		FROM task_session_messages m
		WHERE m.task_session_id = ?` + unreadCandidateFilter
	args := []interface{}{sessionID}
	if cursor.key != nil {
		query += `
		  AND ` + nmMsg + ` >= ?
		  AND (` + nmMsg + ` > ? OR m.id > ?)`
		args = append(args, cursor.key, cursor.key, cursor.rank.ID)
	}
	if boundary.id != "" {
		query += `
		  AND ` + nmMsg + ` <= ?
		  AND (` + nmMsg + ` < ? OR m.id < ?)`
		args = append(args, boundary.key, boundary.key, boundary.id)
	}
	query += `
		ORDER BY ` + nmMsg + ` DESC, m.id DESC
		LIMIT ?`
	args = append(args, unreadWalkPageSize)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("load unread walk page for session %q: %w", sessionID, err)
	}
	defer func() { _ = rows.Close() }()
	page := make([]unreadWalkRow, 0, unreadWalkPageSize)
	for rows.Next() {
		var id, authorType, messageType, content, metadataJSON string
		var rawCreatedAt, key interface{}
		if err := rows.Scan(&id, &authorType, &messageType, &content, &metadataJSON, &rawCreatedAt, &key); err != nil {
			return nil, fmt.Errorf("scan unread walk row for session %q: %w", sessionID, err)
		}
		row, err := buildUnreadWalkRow(id, authorType, messageType, content, metadataJSON, rawCreatedAt, key)
		if err != nil {
			return nil, err
		}
		page = append(page, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read unread walk page for session %q: %w", sessionID, err)
	}
	return page, nil
}

// unreadCandidateFilter excludes rows that can never qualify: user-written
// rows and types outside the transcript-visible set. It is a necessary
// condition of statussummary.IsUnreadQualifyingMessage, so it never drops a
// qualifying row, and it keeps a long run of hidden rows inside one query
// instead of paging them into Go. The values are compile-time constants.
var unreadCandidateFilter = func() string {
	types := statussummary.UnreadCandidateMessageTypes()
	quoted := make([]string, 0, len(types))
	for _, messageType := range types {
		quoted = append(quoted, "'"+strings.ReplaceAll(messageType, "'", "''")+"'")
	}
	return `
				  AND m.author_type <> '` + string(models.MessageAuthorUser) + `'
				  AND m.type IN (` + strings.Join(quoted, ", ") + `)`
}()

func buildUnreadWalkRow(
	id, authorType, messageType, content, metadataJSON string,
	rawCreatedAt, key interface{},
) (unreadWalkRow, error) {
	createdAt, err := parseTaskActivityTime(rawCreatedAt)
	if err != nil {
		return unreadWalkRow{}, fmt.Errorf("parse unread walk time for message %q: %w", id, err)
	}
	var metadata map[string]interface{}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			return unreadWalkRow{}, fmt.Errorf("decode unread walk metadata for message %q: %w", id, err)
		}
	}
	return unreadWalkRow{
		rank:      statussummary.NewUnreadRank(id, createdAt),
		key:       key,
		qualifies: statussummary.IsUnreadQualifyingMessage(authorType, messageType, content, metadata),
	}, nil
}
