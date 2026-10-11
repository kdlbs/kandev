package messagequeue

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// The initial RED used interface discovery to fail on behavior rather than compilation.
// The typed helper now exercises the production page contract directly.
func recoveryPageCall(t *testing.T, service *Service, identity QueueSessionIdentity, cursor string, budget int, expected string) (*QueueRecoveryPage, error) {
	t.Helper()
	return service.RecoverySnapshotPage(context.Background(), identity, "workspace", cursor, budget, expected)
}

// @covers AC-TASKS-QUEUE-RECOVERY-001.4
func TestRecoverySnapshotReconstructsFIFO(t *testing.T) {
	for _, factory := range recoveryFactories() {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			identity := QueueSessionIdentity{"task", "session", "incarnation"}
			seedQueueSessionIdentity(t, repo, identity)
			bodies := []string{"", strings.Repeat("漢🙂", 420), "same", "same"}
			ids := []string{"empty", "long", "duplicate-1", "duplicate-2"}
			for i, body := range bodies {
				require.NoError(t, repo.InsertForSession(context.Background(), identity, &QueuedMessage{ID: ids[i], SessionID: "session", TaskID: "task", Content: body, QueuedAt: time.Now().UTC()}, 0))
			}
			service := NewService(repo, 0, logger.Default())
			assembled := map[string]string{}
			cursor, digest := "", ""
			for pages := 0; ; pages++ {
				require.Less(t, pages, 20, "pagination must progress")
				page, err := recoveryPageCall(t, service, identity, cursor, 1024, digest)
				require.NoError(t, err)
				if digest == "" {
					digest = page.SnapshotSHA256
				}
				require.Equal(t, digest, page.SnapshotSHA256)
				require.LessOrEqual(t, page.PageBodyBytes, 1024)
				for _, fragment := range page.Fragments {
					require.Equal(t, len(assembled[fragment.ID]), fragment.BodyOffsetBytes)
					assembled[fragment.ID] += fragment.Body
				}
				cursor = page.NextCursor
				if cursor == "" {
					break
				}
			}
			for i, id := range ids {
				require.Equal(t, bodies[i], assembled[id])
			}
			require.Len(t, assembled, 4)
		})
	}
}

// @covers AC-TASKS-QUEUE-RECOVERY-001.5
func TestRecoverySnapshotRejectsChangedAndPinnedDigest(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{"task", "session", "incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	require.NoError(t, repo.InsertForSession(context.Background(), identity, &QueuedMessage{ID: "entry", TaskID: "task", SessionID: "session", Content: strings.Repeat("x", 2048), QueuedAt: time.Now().UTC()}, 0))
	service := NewService(repo, 0, logger.Default())
	first, err := recoveryPageCall(t, service, identity, "", 1024, "")
	require.NoError(t, err)
	require.NotEmpty(t, first.NextCursor)
	require.NoError(t, repo.InsertForSession(context.Background(), identity, &QueuedMessage{ID: "new", TaskID: "task", SessionID: "session", Content: "new", QueuedAt: time.Now().UTC()}, 0))
	_, err = recoveryPageCall(t, service, identity, first.NextCursor, 1024, "")
	require.Error(t, err)
	_, err = recoveryPageCall(t, service, identity, "", 1024, first.SnapshotSHA256)
	require.Error(t, err)
}

func TestRecoverySnapshotBodyHash(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{"task", "session", "incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	require.NoError(t, repo.InsertForSession(context.Background(), identity, &QueuedMessage{ID: "entry", TaskID: "task", SessionID: "session", Content: "body", QueuedAt: time.Now().UTC()}, 0))
	page, err := recoveryPageCall(t, NewService(repo, 0, logger.Default()), identity, "", 1024, "")
	require.NoError(t, err)
	hash := sha256.Sum256([]byte("body"))
	require.Equal(t, hex.EncodeToString(hash[:]), page.Fragments[0].ContentSHA256)
}

func TestRecoverySnapshotRejectsChangedEvidence(t *testing.T) {
	mutations := map[string]func(*QueuedMessage){
		"body":        func(e *QueuedMessage) { e.Content += "changed" },
		"model":       func(e *QueuedMessage) { e.Model = "changed" },
		"position":    func(e *QueuedMessage) { e.Position++ },
		"attachment":  func(e *QueuedMessage) { e.Attachments = []MessageAttachment{{Name: "private", Data: "changed"}} },
		"settings":    func(e *QueuedMessage) { e.Metadata = map[string]interface{}{"setting": "changed"} },
		"reservation": func(e *QueuedMessage) { e.Metadata = map[string]interface{}{MetadataLifecycleReserved: true} },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			repo := NewMemoryRepository().(*memoryRepository)
			identity := QueueSessionIdentity{"task", "session", "incarnation"}
			seedQueueSessionIdentity(t, repo, identity)
			require.NoError(t, repo.InsertForSession(context.Background(), identity, &QueuedMessage{ID: "entry", TaskID: "task", SessionID: "session", Content: strings.Repeat("x", 2048), QueuedAt: time.Now().UTC()}, 0))
			service := NewService(repo, 0, logger.Default())
			first, err := recoveryPageCall(t, service, identity, "", 1024, "")
			require.NoError(t, err)
			mutate(repo.entries["session"][0])
			page, err := recoveryPageCall(t, service, identity, first.NextCursor, 1024, "")
			require.ErrorIs(t, err, ErrRecoveryChanged)
			require.Nil(t, page)
		})
	}
}

func TestRecoverySnapshotInvalidCursorAndFragmentCap(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{"task", "session", "incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	for i := range 300 {
		require.NoError(t, repo.InsertForSession(context.Background(), identity, &QueuedMessage{ID: fmt.Sprintf("empty-%d", i), TaskID: "task", SessionID: "session", QueuedAt: time.Now().UTC()}, 0))
	}
	service := NewService(repo, 0, logger.Default())
	first, err := recoveryPageCall(t, service, identity, "", 1024, "")
	require.NoError(t, err)
	require.Len(t, first.Fragments, 256)
	require.NotEmpty(t, first.NextCursor)
	second, err := recoveryPageCall(t, service, identity, first.NextCursor, 1024, "")
	require.NoError(t, err)
	require.Len(t, second.Fragments, 44)
	require.Empty(t, second.NextCursor)
	for _, cursor := range []string{"!malformed!", strings.Repeat("x", 4097)} {
		page, err := recoveryPageCall(t, service, identity, cursor, 1024, "")
		require.ErrorIs(t, err, ErrInvalidRecoveryPage)
		require.Nil(t, page)
	}
	for _, budget := range []int{-1, 1023, 262145} {
		page, err := recoveryPageCall(t, service, identity, "", budget, "")
		require.ErrorIs(t, err, ErrInvalidRecoveryPage)
		require.Nil(t, page)
	}
}

func TestRecoverySnapshotRejectsCursorOffsetAndIdentityForgery(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{"task", "session", "incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	require.NoError(t, repo.InsertForSession(context.Background(), identity, &QueuedMessage{ID: "entry", TaskID: "task", SessionID: "session", Content: "漢🙂", QueuedAt: time.Now().UTC()}, 0))
	service := NewService(repo, 0, logger.Default())
	page, err := recoveryPageCall(t, service, identity, "", 1024, "")
	require.NoError(t, err)
	valid := recoveryCursor{Version: 1, Identity: identity, WorkspaceID: "workspace", Digest: page.SnapshotSHA256}
	for _, mutate := range []func(*recoveryCursor){
		func(c *recoveryCursor) { c.Offset = -1 },
		func(c *recoveryCursor) { c.Offset = 1 },
		func(c *recoveryCursor) { c.Offset = 100 },
		func(c *recoveryCursor) { c.Index = -1 },
		func(c *recoveryCursor) { c.Version = 2 },
		func(c *recoveryCursor) { c.WorkspaceID = "foreign" },
		func(c *recoveryCursor) { c.Identity.SessionID = "foreign" },
	} {
		cursor := valid
		mutate(&cursor)
		encoded, err := json.Marshal(cursor)
		require.NoError(t, err)
		response, err := recoveryPageCall(t, service, identity, base64.RawURLEncoding.EncodeToString(encoded), 1024, "")
		require.ErrorIs(t, err, ErrInvalidRecoveryPage)
		require.Nil(t, response)
	}
}
