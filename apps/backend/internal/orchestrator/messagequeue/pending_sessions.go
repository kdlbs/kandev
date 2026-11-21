package messagequeue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// errPendingSessionListingUnsupported is returned by repositories that cannot
// enumerate sessions with queued rows.
var errPendingSessionListingUnsupported = errors.New("pending session listing is not supported")

type pendingSessionLister interface {
	ListSessionsWithPending(ctx context.Context) ([]string, error)
}

// ListSessionsWithPending returns the sessions that hold at least one queued
// row that is not reserved by an in-flight dispatch.
func (s *Service) ListSessionsWithPending(ctx context.Context) ([]string, error) {
	lister, ok := s.repo.(pendingSessionLister)
	if !ok {
		return nil, errPendingSessionListingUnsupported
	}
	return lister.ListSessionsWithPending(ctx)
}

func (r *memoryRepository) ListSessionsWithPending(_ context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for sessionID, list := range r.entries {
		for _, msg := range list {
			if !msg.IsReservedInFlight() {
				out = append(out, sessionID)
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func (r *sqliteRepository) ListSessionsWithPending(ctx context.Context) ([]string, error) {
	// Only the session and its reservation marker are needed; message bodies
	// and attachments are neither read nor decoded on every sweep.
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT q.session_id, q.metadata_json
		FROM queued_messages q
		INNER JOIN task_sessions s ON s.id = q.session_id
		ORDER BY q.session_id ASC, q.position ASC
	`))
	if err != nil {
		return nil, fmt.Errorf("list sessions with pending: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var sessionID, metaJSON string
		if scanErr := rows.Scan(&sessionID, &metaJSON); scanErr != nil {
			return nil, fmt.Errorf("list sessions with pending scan: %w", scanErr)
		}
		if len(out) > 0 && out[len(out)-1] == sessionID {
			continue
		}
		reserved, decodeErr := metadataJSONReservedInFlight(metaJSON)
		if decodeErr != nil {
			return nil, fmt.Errorf("list sessions with pending scan: %w", decodeErr)
		}
		if !reserved {
			out = append(out, sessionID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sessions with pending rows: %w", err)
	}
	return out, nil
}

// metadataJSONReservedInFlight mirrors QueuedMessage.IsReservedInFlight for a
// stored metadata_json value.
func metadataJSONReservedInFlight(metaJSON string) (bool, error) {
	if metaJSON == "" || metaJSON == "{}" {
		return false, nil
	}
	msg := QueuedMessage{}
	if err := json.Unmarshal([]byte(metaJSON), &msg.Metadata); err != nil {
		return false, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return msg.IsReservedInFlight(), nil
}
