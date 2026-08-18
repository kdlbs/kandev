package messagequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/messageconstraints"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type automaticMergeRepository interface {
	AutoMergeIntoAbove(context.Context, string, string) (*QueuedMessage, bool, error)
}

type automaticMergeCandidateRepository interface {
	AutoMergeCandidateIntoAbove(context.Context, *QueuedMessage) (*QueuedMessage, bool, error)
}

type autoMergeRepositoryFactory struct {
	name string
	new  func(*testing.T) Repository
}

func autoMergeRepositoryFactories() []autoMergeRepositoryFactory {
	return []autoMergeRepositoryFactory{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
		{name: "postgres", new: newTestPostgresRepo},
	}
}

func TestRepository_AutoMergeIntoAbove_UserHappyPath(t *testing.T) {
	for _, tt := range autoMergeRepositoryFactories() {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			autoRepo, ok := repo.(automaticMergeRepository)
			if !ok {
				t.Fatalf("repository %T does not implement automatic tail merge", repo)
			}

			target := insertTestEntry(t, repo, "session", "task", "first", QueuedByUser, nil, nil)
			source := insertTestEntry(t, repo, "session", "task", "second", QueuedByUser, nil, nil)

			merged, didMerge, err := autoRepo.AutoMergeIntoAbove(context.Background(), "session", source.ID)
			if err != nil {
				t.Fatalf("auto merge: %v", err)
			}
			if !didMerge {
				t.Fatal("auto merge skipped compatible entries")
			}
			if merged == nil || merged.ID != target.ID {
				t.Fatalf("survivor = %+v, want target %s", merged, target.ID)
			}
			if merged.Content != "first\n\nsecond" {
				t.Errorf("content = %q, want %q", merged.Content, "first\n\nsecond")
			}

			entries, err := repo.ListBySession(context.Background(), "session")
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(entries) != 1 || entries[0].ID != target.ID {
				t.Fatalf("entries after merge = %+v, want surviving target", entries)
			}
		})
	}
}

func TestRepository_AutoMergeIntoAbove_PreservesIdentityAndCombinesAdditiveData(t *testing.T) {
	for _, tt := range autoMergeRepositoryFactories() {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			autoRepo := repo.(automaticMergeRepository)
			isDirectory := true
			target := insertAutoMergeEntry(t, repo, QueuedMessage{
				SessionID: "session", TaskID: "task", Content: "first", Model: "model", PlanMode: true, QueuedBy: "alice",
				Attachments: []MessageAttachment{{AttachmentID: "target-file", Name: "target.txt", SizeBytes: 6}},
				Metadata: map[string]interface{}{
					"route":                  map[string]interface{}{"attempt": 1, "tags": []string{"one"}},
					MetadataEntityReferences: entityRefs("1"),
					MetadataContextFiles:     []apiv1.ContextFileMeta{{Path: "a.go", Name: "a.go"}},
				},
			})
			source := insertAutoMergeEntry(t, repo, QueuedMessage{
				SessionID: "session", TaskID: "task", Content: "second", Model: "model", PlanMode: true, QueuedBy: "alice",
				Attachments: []MessageAttachment{{Data: "c291cmNl", Name: "source.txt"}},
				Metadata: map[string]interface{}{
					"route":                  map[string]interface{}{"attempt": float64(1), "tags": []interface{}{"one"}},
					MetadataEntityReferences: entityRefs("1", "2"),
					MetadataContextFiles: []interface{}{
						map[string]interface{}{"path": "a.go", "name": "a.go"},
						map[string]interface{}{"path": "dir", "name": "dir", "is_directory": isDirectory},
					},
				},
			})
			targetIdentity := autoMergeIdentity(*target)
			// The surviving row carries the newest admission timestamp so durable
			// activity reconstruction does not lose the folded user prompt.
			targetIdentity.QueuedAt = source.QueuedAt.UTC().Truncate(time.Microsecond)

			merged, didMerge, err := autoRepo.AutoMergeIntoAbove(context.Background(), "session", source.ID)
			if err != nil || !didMerge {
				t.Fatalf("auto merge: merged=%v err=%v", didMerge, err)
			}
			if got := autoMergeIdentity(*merged); !reflect.DeepEqual(got, targetIdentity) {
				t.Errorf("survivor identity changed: got %+v want %+v", got, targetIdentity)
			}
			if merged.Content != "first\n\nsecond" {
				t.Errorf("content = %q", merged.Content)
			}
			if got := attachmentNames(merged.Attachments); !reflect.DeepEqual(got, []string{"target.txt", "source.txt"}) {
				t.Errorf("attachment order = %v", got)
			}
			if got := referenceIDs(merged.Metadata); !reflect.DeepEqual(got, []string{"1", "2"}) {
				t.Errorf("reference order = %v", got)
			}
			contexts, ok := normalizeAutoContextFiles(merged.Metadata[MetadataContextFiles])
			if !ok || len(contexts) != 2 || contexts[0].Path != "a.go" || contexts[1].Path != "dir" {
				t.Errorf("context union = %+v, valid=%v", contexts, ok)
			}
		})
	}
}

func TestRepository_AutoMergeIntoAbove_AgentSameSender(t *testing.T) {
	for _, tt := range autoMergeRepositoryFactories() {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			targetMessage := defaultAutoMergeEntry("agent", "first")
			targetMessage.QueuedBy = QueuedByAgent
			targetMessage.Metadata = senderMetadata("sender-task")
			target := insertAutoMergeEntry(t, repo, targetMessage)
			sourceMessage := defaultAutoMergeEntry("agent", "second")
			sourceMessage.QueuedBy = QueuedByAgent
			sourceMessage.Metadata = senderMetadata("sender-task")
			source := insertAutoMergeEntry(t, repo, sourceMessage)

			merged, didMerge, err := repo.(automaticMergeRepository).AutoMergeIntoAbove(context.Background(), "agent", source.ID)
			if err != nil || !didMerge || merged == nil {
				t.Fatalf("agent auto merge: result=%+v merged=%v err=%v", merged, didMerge, err)
			}
			if merged.ID != target.ID || metadataString(merged.Metadata, MetadataSenderTaskID) != "sender-task" {
				t.Fatalf("agent survivor = %+v, want target identity and sender", merged)
			}
		})
	}
}

func TestRepository_AutoMergeIntoAbove_IncompatibilityLeavesRowsUntouched(t *testing.T) {
	for _, tt := range autoMergeRepositoryFactories() {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			for _, test := range autoMergeSkipCases() {
				t.Run(test.name, func(t *testing.T) {
					runAutoMergeSkipCase(t, repo, tt.name+"-"+test.name, test.mutate)
				})
			}
		})
	}
}

func TestRepository_AutoMergeIntoAbove_UsesOnlyImmediatePredecessor(t *testing.T) {
	for _, tt := range autoMergeRepositoryFactories() {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			autoRepo := repo.(automaticMergeRepository)
			insertAutoMergeEntry(t, repo, defaultAutoMergeEntry("immediate", "compatible-before"))
			blocker := defaultAutoMergeEntry("immediate", "blocker")
			blocker.QueuedBy = "bob"
			insertAutoMergeEntry(t, repo, blocker)
			source := insertAutoMergeEntry(t, repo, defaultAutoMergeEntry("immediate", "source"))
			before := autoQueueSnapshot(t, repo, "immediate")

			result, merged, err := autoRepo.AutoMergeIntoAbove(context.Background(), "immediate", source.ID)
			if err != nil || merged {
				t.Fatalf("immediate-tail skip: merged=%v err=%v", merged, err)
			}
			if result == nil || result.ID != source.ID {
				t.Fatalf("skip result = %+v, want source", result)
			}
			if after := autoQueueSnapshot(t, repo, "immediate"); after != before {
				t.Fatalf("queue changed on skip\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func TestRepository_AutoMergeIntoAbove_MissingAndHeadAreSafeSkips(t *testing.T) {
	for _, tt := range autoMergeRepositoryFactories() {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			autoRepo := repo.(automaticMergeRepository)
			head := insertAutoMergeEntry(t, repo, defaultAutoMergeEntry("missing", "head"))
			result, merged, err := autoRepo.AutoMergeIntoAbove(context.Background(), "missing", head.ID)
			if err != nil || merged || result == nil || result.ID != head.ID {
				t.Fatalf("head skip: result=%+v merged=%v err=%v", result, merged, err)
			}
			source := insertAutoMergeEntry(t, repo, defaultAutoMergeEntry("missing", "source"))
			if _, err := repo.TakeByID(context.Background(), "missing", source.ID); err != nil {
				t.Fatalf("drain source: %v", err)
			}
			before := autoQueueSnapshot(t, repo, "missing")
			result, merged, err = autoRepo.AutoMergeIntoAbove(context.Background(), "missing", source.ID)
			if err != nil || merged || result != nil {
				t.Fatalf("missing skip: result=%+v merged=%v err=%v", result, merged, err)
			}
			if after := autoQueueSnapshot(t, repo, "missing"); after != before {
				t.Fatalf("queue changed after missing-source skip")
			}
		})
	}
}

func TestRepository_AutoMergeIntoAbove_EmptyContentHasNoExtraSeparator(t *testing.T) {
	tests := []struct {
		name, target, source, want string
	}{
		{name: "empty target", source: "source", want: "source"},
		{name: "empty source", target: "target", want: "target"},
		{name: "both empty", want: ""},
	}
	for _, tt := range autoMergeRepositoryFactories() {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			autoRepo := repo.(automaticMergeRepository)
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					session := "empty-" + test.name
					insertAutoMergeEntry(t, repo, defaultAutoMergeEntry(session, test.target))
					source := insertAutoMergeEntry(t, repo, defaultAutoMergeEntry(session, test.source))
					merged, didMerge, err := autoRepo.AutoMergeIntoAbove(context.Background(), session, source.ID)
					if err != nil || !didMerge || merged.Content != test.want {
						t.Fatalf("result=%+v merged=%v err=%v, want content %q", merged, didMerge, err, test.want)
					}
				})
			}
		})
	}
}

func TestRepository_AutoMergeCandidateIntoAbove_FoldsIntoTail(t *testing.T) {
	for _, tt := range autoMergeRepositoryFactories() {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			autoRepo, ok := repo.(automaticMergeCandidateRepository)
			if !ok {
				t.Fatalf("repository %T does not implement candidate tail merge", repo)
			}

			insertAutoMergeEntry(t, repo, defaultAutoMergeEntry("candidate", "first"))
			tail := insertAutoMergeEntry(t, repo, defaultAutoMergeEntry("candidate", "second"))
			candidate := defaultAutoMergeEntry("candidate", "third")

			merged, didMerge, err := autoRepo.AutoMergeCandidateIntoAbove(context.Background(), &candidate)
			if err != nil || !didMerge || merged == nil {
				t.Fatalf("candidate merge: result=%+v merged=%v err=%v", merged, didMerge, err)
			}
			if merged.ID != tail.ID {
				t.Fatalf("survivor = %s, want tail %s", merged.ID, tail.ID)
			}
			if merged.Content != "second\n\nthird" {
				t.Errorf("content = %q, want %q", merged.Content, "second\n\nthird")
			}
			entries, err := repo.ListBySession(context.Background(), "candidate")
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(entries) != 2 || entries[1].ID != tail.ID {
				t.Fatalf("entries after candidate fold = %+v, want two rows with folded tail", entries)
			}
		})
	}
}

func TestRepository_AutoMergeCandidateIntoAbove_IncompatibleSkips(t *testing.T) {
	for _, tt := range autoMergeRepositoryFactories() {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			autoRepo, ok := repo.(automaticMergeCandidateRepository)
			if !ok {
				t.Fatalf("repository %T does not implement candidate tail merge", repo)
			}
			insertAutoMergeEntry(t, repo, defaultAutoMergeEntry("candidate-skip", "first"))
			candidate := defaultAutoMergeEntry("candidate-skip", "other-model")
			candidate.Model = "other-model"
			before := autoQueueSnapshot(t, repo, "candidate-skip")

			merged, didMerge, err := autoRepo.AutoMergeCandidateIntoAbove(context.Background(), &candidate)
			if err != nil || didMerge || merged != nil {
				t.Fatalf("incompatible candidate: result=%+v merged=%v err=%v", merged, didMerge, err)
			}
			if after := autoQueueSnapshot(t, repo, "candidate-skip"); after != before {
				t.Fatalf("queue mutated on skip\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func TestRepository_AutoMergeCandidateIntoAbove_EmptyQueueSkips(t *testing.T) {
	for _, tt := range autoMergeRepositoryFactories() {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			autoRepo, ok := repo.(automaticMergeCandidateRepository)
			if !ok {
				t.Fatalf("repository %T does not implement candidate tail merge", repo)
			}
			candidate := defaultAutoMergeEntry("candidate-empty", "only")
			merged, didMerge, err := autoRepo.AutoMergeCandidateIntoAbove(context.Background(), &candidate)
			if err != nil || didMerge || merged != nil {
				t.Fatalf("empty queue: result=%+v merged=%v err=%v", merged, didMerge, err)
			}
		})
	}
}

func TestSQLiteRepository_AutoMergeCandidateIntoAbove_WriteFailureRollsBack(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	db := repo.(*sqliteRepository).db
	insertAutoMergeEntry(t, repo, defaultAutoMergeEntry("candidate-rollback", "first"))
	insertAutoMergeEntry(t, repo, defaultAutoMergeEntry("candidate-rollback", "second"))
	before := autoQueueSnapshot(t, repo, "candidate-rollback")
	if _, err := db.Exec(`
		CREATE TRIGGER fail_auto_merge_candidate_update
		BEFORE UPDATE ON queued_messages
		BEGIN
			SELECT RAISE(ABORT, 'forced automatic candidate merge failure');
		END;
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	candidate := defaultAutoMergeEntry("candidate-rollback", "third")
	merged, didMerge, err := repo.(automaticMergeCandidateRepository).AutoMergeCandidateIntoAbove(context.Background(), &candidate)
	if err == nil || didMerge || merged != nil {
		t.Fatalf("forced write failure: result=%+v merged=%v err=%v", merged, didMerge, err)
	}
	if after := autoQueueSnapshot(t, repo, "candidate-rollback"); after != before {
		t.Fatalf("write failure was not atomic\nbefore=%s\nafter=%s", before, after)
	}
}

func TestSQLiteRepository_AutoMergeIntoAbove_WriteFailureRollsBack(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	testAutoMergeWriteRollback(t, repo, `
		CREATE TRIGGER fail_auto_merge_delete
		BEFORE DELETE ON queued_messages
		BEGIN
			SELECT RAISE(ABORT, 'forced automatic merge failure');
		END;
	`)
}

func TestPostgresRepository_AutoMergeIntoAbove_WriteFailureRollsBack(t *testing.T) {
	repo := newTestPostgresRepo(t)
	db := repo.(*sqliteRepository).db
	if _, err := db.Exec(`
		CREATE FUNCTION fail_auto_merge_delete() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'forced automatic merge failure';
		END;
		$$ LANGUAGE plpgsql
	`); err != nil {
		t.Fatalf("create failure function: %v", err)
	}
	testAutoMergeWriteRollback(t, repo, `
		CREATE TRIGGER fail_auto_merge_delete
		BEFORE DELETE ON queued_messages
		FOR EACH ROW EXECUTE FUNCTION fail_auto_merge_delete()
	`)
}

type autoMergeIdentityFields struct {
	ID        string
	SessionID string
	TaskID    string
	Position  int64
	Model     string
	PlanMode  bool
	QueuedBy  string
	QueuedAt  time.Time
}

func autoMergeIdentity(message QueuedMessage) autoMergeIdentityFields {
	return autoMergeIdentityFields{
		ID: message.ID, SessionID: message.SessionID, TaskID: message.TaskID,
		Position: message.Position, Model: message.Model, PlanMode: message.PlanMode,
		QueuedBy: message.QueuedBy, QueuedAt: message.QueuedAt.UTC().Truncate(time.Microsecond),
	}
}

func insertAutoMergeEntry(t *testing.T, repo Repository, message QueuedMessage) *QueuedMessage {
	t.Helper()
	if err := repo.Insert(context.Background(), &message, 0); err != nil {
		t.Fatalf("insert auto-merge entry: %v", err)
	}
	return &message
}

func defaultAutoMergeEntry(session, content string) QueuedMessage {
	return QueuedMessage{
		SessionID: session, TaskID: "task", Content: content, Model: "model", QueuedBy: QueuedByUser,
	}
}

func attachmentNames(attachments []MessageAttachment) []string {
	names := make([]string, len(attachments))
	for index := range attachments {
		names[index] = attachments[index].Name
	}
	return names
}

func referenceIDs(metadata map[string]interface{}) []string {
	references := entityrefs.NormalizePersisted(metadata[MetadataEntityReferences])
	ids := make([]string, len(references))
	for index := range references {
		ids[index] = references[index].ID
	}
	return ids
}

func autoQueueSnapshot(t *testing.T, repo Repository, session string) string {
	t.Helper()
	entries, err := repo.ListBySession(context.Background(), session)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal queue snapshot: %v", err)
	}
	return string(encoded)
}

type autoMergeSkipCase struct {
	name   string
	mutate func(*QueuedMessage, *QueuedMessage)
}

func autoMergeSkipCases() []autoMergeSkipCase {
	return []autoMergeSkipCase{
		{name: "task mismatch", mutate: func(_, source *QueuedMessage) { source.TaskID = "other-task" }},
		{name: "model mismatch", mutate: func(_, source *QueuedMessage) { source.Model = "other-model" }},
		{name: "plan mode mismatch", mutate: func(_, source *QueuedMessage) { source.PlanMode = true }},
		{name: "user identity mismatch", mutate: func(_, source *QueuedMessage) { source.QueuedBy = "bob" }},
		{name: "user agent mismatch", mutate: func(_, source *QueuedMessage) {
			source.QueuedBy = QueuedByAgent
			source.Metadata = senderMetadata("sender")
		}},
		{name: "agent clarification missing sender", mutate: func(target, source *QueuedMessage) { target.QueuedBy = QueuedByAgent; source.QueuedBy = QueuedByAgent }},
		{name: "agent sender mismatch", mutate: func(target, source *QueuedMessage) {
			target.QueuedBy, source.QueuedBy = QueuedByAgent, QueuedByAgent
			target.Metadata, source.Metadata = senderMetadata("one"), senderMetadata("two")
		}},
		{name: "agent session mismatch", mutate: func(target, source *QueuedMessage) {
			// Different sessions of the same sender task are different agents
			// and execution contexts; auto-merge must keep their prompts
			// separate even though manual merge would allow the fold.
			target.QueuedBy, source.QueuedBy = QueuedByAgent, QueuedByAgent
			target.Metadata = map[string]interface{}{
				"sender_task_id": "sender-task", "sender_session_id": "session-one",
				"sender_session_name": "reviewer-one", "sender_task_title": "Sender task",
			}
			source.Metadata = map[string]interface{}{
				"sender_task_id": "sender-task", "sender_session_id": "session-two",
				"sender_session_name": "reviewer-two", "sender_task_title": "Sender task",
			}
		}},
		{name: "workflow source", mutate: func(target, source *QueuedMessage) {
			target.QueuedBy, source.QueuedBy = QueuedByWorkflow, QueuedByWorkflow
		}},
		{name: "server source", mutate: func(target, source *QueuedMessage) { target.QueuedBy, source.QueuedBy = QueuedByServer, QueuedByServer }},
		{name: "durable target", mutate: func(target, source *QueuedMessage) {
			target.Metadata, source.Metadata = durableMetadata(), durableMetadata()
		}},
		{name: "durable source", mutate: func(_, source *QueuedMessage) { source.Metadata = durableMetadata() }},
		{name: "metadata mismatch", mutate: func(target, source *QueuedMessage) {
			target.Metadata = map[string]interface{}{"plugin_source": "one"}
			source.Metadata = map[string]interface{}{"plugin_source": "two"}
		}},
		{name: "attachment count overflow", mutate: func(target, source *QueuedMessage) {
			target.Attachments = autoFileAttachments("target", messageconstraints.MaxAttachmentCount/2+1, 1)
			source.Attachments = autoFileAttachments("source", messageconstraints.MaxAttachmentCount/2, 1)
		}},
		{name: "attachment bytes overflow", mutate: func(target, source *QueuedMessage) {
			target.Attachments = autoFileAttachments("target", 1, messageconstraints.MaxAttachmentBytes/2+1)
			source.Attachments = autoFileAttachments("source", 1, messageconstraints.MaxAttachmentBytes/2)
		}},
		{name: "entity reference overflow", mutate: func(target, source *QueuedMessage) {
			target.Metadata = map[string]interface{}{MetadataEntityReferences: manyEntityRefs(entityrefs.MaxReferencesPerMessage)}
			source.Metadata = map[string]interface{}{MetadataEntityReferences: entityRefs("overflow")}
		}},
		{name: "context file overflow", mutate: func(target, source *QueuedMessage) {
			target.Metadata = map[string]interface{}{MetadataContextFiles: autoContextFiles("target", messageconstraints.MaxContextFilesPerMessage/2+1)}
			source.Metadata = map[string]interface{}{MetadataContextFiles: autoContextFiles("source", messageconstraints.MaxContextFilesPerMessage/2)}
		}},
		{name: "malformed context files", mutate: func(target, source *QueuedMessage) {
			target.Metadata = map[string]interface{}{MetadataContextFiles: []interface{}{map[string]interface{}{"path": "valid", "name": "valid"}}}
			source.Metadata = map[string]interface{}{MetadataContextFiles: "invalid"}
		}},
	}
}

func runAutoMergeSkipCase(t *testing.T, repo Repository, session string, mutate func(*QueuedMessage, *QueuedMessage)) {
	t.Helper()
	target := defaultAutoMergeEntry(session, "target")
	source := defaultAutoMergeEntry(session, "source")
	mutate(&target, &source)
	insertAutoMergeEntry(t, repo, target)
	insertedSource := insertAutoMergeEntry(t, repo, source)
	before := autoQueueSnapshot(t, repo, session)
	result, merged, err := repo.(automaticMergeRepository).AutoMergeIntoAbove(context.Background(), session, insertedSource.ID)
	if err != nil || merged {
		t.Fatalf("skip result: merged=%v err=%v", merged, err)
	}
	if result == nil || result.ID != insertedSource.ID {
		t.Fatalf("skip returned %+v, want source %s", result, insertedSource.ID)
	}
	if after := autoQueueSnapshot(t, repo, session); after != before {
		t.Fatalf("queue mutated on skip\nbefore=%s\nafter=%s", before, after)
	}
}

func senderMetadata(sender string) map[string]interface{} {
	return map[string]interface{}{MetadataSenderTaskID: sender}
}

func durableMetadata() map[string]interface{} {
	return map[string]interface{}{MetadataLifecycleDurable: true}
}

func autoFileAttachments(prefix string, count int, size int64) []MessageAttachment {
	attachments := make([]MessageAttachment, count)
	for index := range attachments {
		attachments[index] = MessageAttachment{AttachmentID: prefix + string(rune('a'+index)), SizeBytes: size}
	}
	return attachments
}

func autoContextFiles(prefix string, count int) []apiv1.ContextFileMeta {
	files := make([]apiv1.ContextFileMeta, count)
	for index := range files {
		files[index] = apiv1.ContextFileMeta{Path: prefix + "/file-" + fmt.Sprint(index), Name: "file"}
	}
	return files
}

func testAutoMergeWriteRollback(t *testing.T, repo Repository, createTrigger string) {
	t.Helper()
	db := repo.(*sqliteRepository).db
	target := insertAutoMergeEntry(t, repo, defaultAutoMergeEntry("rollback", "target"))
	source := insertAutoMergeEntry(t, repo, defaultAutoMergeEntry("rollback", "source"))
	before := autoQueueSnapshot(t, repo, "rollback")
	if _, err := db.Exec(createTrigger); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	result, merged, err := repo.(automaticMergeRepository).AutoMergeIntoAbove(context.Background(), "rollback", source.ID)
	if err == nil || merged || result != nil {
		t.Fatalf("forced write failure: result=%+v merged=%v err=%v", result, merged, err)
	}
	if after := autoQueueSnapshot(t, repo, "rollback"); after != before {
		t.Fatalf("write failure was not atomic\nbefore=%s\nafter=%s", before, after)
	}
	entries, _ := repo.ListBySession(context.Background(), "rollback")
	if len(entries) != 2 || entries[0].ID != target.ID || entries[1].ID != source.ID {
		t.Fatalf("rollback entries = %+v", entries)
	}
}
