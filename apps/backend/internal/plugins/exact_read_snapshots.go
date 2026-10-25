package plugins

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	exactSnapshotTTL      = 5 * time.Minute
	exactSnapshotMaxCount = 32
	exactSnapshotMaxRows  = 5000
	exactSnapshotMaxBytes = 8 << 20
)

type exactReadIdentity struct {
	installationID     string
	workspaceID        string
	method             string
	filterDigest       string
	capabilityRevision uint64
}

type exactReadSnapshot struct {
	identity   exactReadIdentity
	version    string
	observedAt string
	createdAt  time.Time
	rows       []json.RawMessage
	bytes      int
}

type exactReadSnapshotStore struct {
	mu        sync.Mutex
	key       [32]byte
	snapshots map[string]*exactReadSnapshot
	now       func() time.Time
}

func newExactReadSnapshotStore() (*exactReadSnapshotStore, error) {
	store := &exactReadSnapshotStore{snapshots: make(map[string]*exactReadSnapshot), now: time.Now}
	if _, err := rand.Read(store.key[:]); err != nil {
		return nil, err
	}
	return store, nil
}

func (h *pluginHost) exactReadStore() (*exactReadSnapshotStore, error) {
	h.exactSnapshotMu.Lock()
	defer h.exactSnapshotMu.Unlock()
	if h.exactSnapshots != nil {
		return h.exactSnapshots, nil
	}
	store, err := newExactReadSnapshotStore()
	if err != nil {
		return nil, status.Error(codes.Unavailable, "exact read snapshots are unavailable")
	}
	h.exactSnapshots = store
	return store, nil
}

//nolint:cyclop,nestif // Cursor validation and snapshot paging share one immutable read boundary.
func (s *exactReadSnapshotStore) page(
	identity exactReadIdentity,
	requestID string,
	page pluginsdk.ExactReadPage,
	load func() (any, error),
) ([]json.RawMessage, pluginsdk.ExactReadPageInfo, error) {
	limit := normalizePageLimit(page.Limit)
	var snapshot *exactReadSnapshot
	offset := 0
	if page.SnapshotVersion == "" {
		if page.Cursor != "" {
			return nil, pluginsdk.ExactReadPageInfo{}, status.Error(codes.InvalidArgument, "exact read cursor requires a snapshot version")
		}
		value, err := load()
		if err != nil {
			return nil, pluginsdk.ExactReadPageInfo{}, err
		}
		rows, size, err := encodeExactSnapshotRows(value)
		if err != nil {
			return nil, pluginsdk.ExactReadPageInfo{}, err
		}
		if len(rows) > exactSnapshotMaxRows || size > exactSnapshotMaxBytes {
			return nil, pluginsdk.ExactReadPageInfo{}, status.Error(codes.ResourceExhausted, "exact read snapshot exceeds its bounded size")
		}
		now := s.now().UTC()
		snapshot = &exactReadSnapshot{
			identity: identity, version: uuid.NewString(), observedAt: now.Format(time.RFC3339Nano),
			createdAt: now, rows: rows, bytes: size,
		}
		s.mu.Lock()
		s.evictExpiredLocked(now)
		if len(s.snapshots) >= exactSnapshotMaxCount {
			s.evictOldestLocked()
		}
		s.snapshots[snapshot.version] = snapshot
		s.mu.Unlock()
	} else {
		if page.Cursor != "" {
			version, cursorOffset, err := s.decodeCursor(page.Cursor)
			if err != nil || version != page.SnapshotVersion {
				return nil, pluginsdk.ExactReadPageInfo{}, status.Error(codes.InvalidArgument, "exact read cursor is invalid")
			}
			offset = cursorOffset
		}
		s.mu.Lock()
		s.evictExpiredLocked(s.now().UTC())
		snapshot = s.snapshots[page.SnapshotVersion]
		s.mu.Unlock()
		if snapshot == nil {
			return nil, pluginsdk.ExactReadPageInfo{}, status.Error(codes.FailedPrecondition, "exact read snapshot is stale; start a new query")
		}
		if snapshot.identity != identity {
			return nil, pluginsdk.ExactReadPageInfo{}, status.Error(codes.FailedPrecondition, "exact read snapshot does not match this query")
		}
	}
	if offset < 0 || offset > len(snapshot.rows) {
		return nil, pluginsdk.ExactReadPageInfo{}, status.Error(codes.InvalidArgument, "exact read cursor is out of range")
	}
	end := offset + limit
	if end > len(snapshot.rows) {
		end = len(snapshot.rows)
	}
	hasMore := end < len(snapshot.rows)
	info := pluginsdk.ExactReadPageInfo{
		HasMore: hasMore, SnapshotVersion: snapshot.version, ObservedAt: snapshot.observedAt,
		Receipt: pluginsdk.HostReadReceipt{
			InstallationID: identity.installationID, WorkspaceID: identity.workspaceID,
			CapabilityRevision: identity.capabilityRevision, RequestID: requestID,
			SnapshotVersion: snapshot.version, ObservedAt: snapshot.observedAt,
		},
	}
	if hasMore {
		info.NextCursor = s.encodeCursor(snapshot.version, end)
	}
	return append([]json.RawMessage{}, snapshot.rows[offset:end]...), info, nil
}

func encodeExactSnapshotRows(value any) ([]json.RawMessage, int, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, 0, status.Error(codes.Internal, "exact read snapshot could not be encoded")
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(encoded, &rows); err != nil {
		return nil, 0, status.Error(codes.Internal, "exact read snapshot is not a row list")
	}
	return rows, len(encoded), nil
}

func decodeExactSnapshotRows[T any](rows []json.RawMessage) ([]T, error) {
	items := make([]T, len(rows))
	for i, row := range rows {
		if err := json.Unmarshal(row, &items[i]); err != nil {
			return nil, status.Error(codes.Internal, "exact read snapshot could not be decoded")
		}
	}
	return items, nil
}

func (s *exactReadSnapshotStore) encodeCursor(version string, offset int) string {
	payload := version + ":" + strconv.Itoa(offset)
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload + ":" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))))
}

func (s *exactReadSnapshotStore) decodeCursor(cursor string) (string, int, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", 0, err
	}
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 3 || parts[0] == "" {
		return "", 0, fmt.Errorf("invalid cursor shape")
	}
	offset, err := strconv.Atoi(parts[1])
	if err != nil || offset < 0 {
		return "", 0, fmt.Errorf("invalid cursor offset")
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", 0, err
	}
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte(parts[0] + ":" + parts[1]))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return "", 0, fmt.Errorf("cursor signature mismatch")
	}
	return parts[0], offset, nil
}

func (s *exactReadSnapshotStore) evictExpiredLocked(now time.Time) {
	for version, snapshot := range s.snapshots {
		if now.Sub(snapshot.createdAt) > exactSnapshotTTL {
			delete(s.snapshots, version)
		}
	}
}

func (s *exactReadSnapshotStore) evictOldestLocked() {
	versions := make([]string, 0, len(s.snapshots))
	for version := range s.snapshots {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		return s.snapshots[versions[i]].createdAt.Before(s.snapshots[versions[j]].createdAt)
	})
	if len(versions) > 0 {
		delete(s.snapshots, versions[0])
	}
}

func exactFilterDigest(filter any) string {
	encoded, err := json.Marshal(filter)
	if err != nil {
		return "invalid"
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:])
}
