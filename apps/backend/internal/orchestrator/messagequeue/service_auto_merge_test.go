package messagequeue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

type automaticMergeSettings interface {
	AutoMergeEnabled() bool
	SetAutoMergeEnabled(bool)
}

func TestService_AutoMergeCanBeDisabledWithoutChangingManualMerge(t *testing.T) {
	for _, test := range []struct {
		name, auto, manual string
		wantCount          int
	}{
		{name: "both on", auto: "on", manual: "on", wantCount: 1},
		{name: "auto on manual off", auto: "on", manual: "off", wantCount: 1},
		{name: "auto off manual on", auto: "off", manual: "on", wantCount: 2},
		{name: "both off", auto: "off", manual: "off", wantCount: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := newAutoMergeTestService(t, 10)
			svc.SetAutoMergeEnabled(test.auto == "on")
			svc.SetMergeEnabled(test.manual == "on")
			first, err := svc.QueueMessage(context.Background(), "session", "task", "first", "", QueuedByUser, false, nil)
			if err != nil {
				t.Fatalf("queue first: %v", err)
			}
			second, err := svc.QueueMessage(context.Background(), "session", "task", "second", "", QueuedByUser, false, nil)
			if err != nil {
				t.Fatalf("queue second: %v", err)
			}
			if got := svc.GetStatus(context.Background(), "session").Count; got != test.wantCount {
				t.Fatalf("queue count = %d, want %d", got, test.wantCount)
			}
			if test.auto == "on" && second.ID != first.ID {
				t.Errorf("automatic survivor = %s, want %s", second.ID, first.ID)
			}
			if svc.MergeEnabled() != (test.manual == "on") {
				t.Errorf("manual merge setting changed")
			}
		})
	}
}

func TestService_AutoMergeChainsThreeAdmissions(t *testing.T) {
	svc := newAutoMergeTestService(t, 10)
	var survivor string
	for _, content := range []string{"first", "second", "third"} {
		queued, err := svc.QueueMessage(context.Background(), "session", "task", content, "", QueuedByUser, false, nil)
		if err != nil {
			t.Fatalf("queue %q: %v", content, err)
		}
		if survivor == "" {
			survivor = queued.ID
		} else if queued.ID != survivor {
			t.Fatalf("survivor changed to %s, want %s", queued.ID, survivor)
		}
	}
	status := svc.GetStatus(context.Background(), "session")
	if status.Count != 1 || status.Entries[0].Content != "first\n\nsecond\n\nthird" {
		t.Fatalf("chained status = %+v", status)
	}
}

func TestService_AutoMergeKeepsDifferentSenderSessionsSeparate(t *testing.T) {
	svc := newAutoMergeTestService(t, 10)
	sender := func(session string) map[string]interface{} {
		return map[string]interface{}{
			"sender_task_id": "sender-task", "sender_session_id": session,
			"sender_session_name": "reviewer-" + session, "sender_task_title": "Sender task",
		}
	}
	first, err := svc.QueueMessageWithMetadata(context.Background(), "session", "task", "first", "", QueuedByAgent, false, nil, sender("one"))
	if err != nil {
		t.Fatalf("queue first: %v", err)
	}
	// A different session of the same sender task is a different agent and
	// execution context; its prompt must stay a separate queue entry.
	second, err := svc.QueueMessageWithMetadata(context.Background(), "session", "task", "second", "", QueuedByAgent, false, nil, sender("two"))
	if err != nil {
		t.Fatalf("queue second: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("different sender sessions must stay separate, got a fold")
	}
	status := svc.GetStatus(context.Background(), "session")
	if status.Count != 2 {
		t.Fatalf("status = %+v, want two separate entries", status)
	}
}

func TestService_AutoMergeFoldsCompatibleAdmissionAtCapacity(t *testing.T) {
	svc := newAutoMergeTestService(t, 1)
	first, err := svc.QueueMessage(context.Background(), "session", "task", "first", "", QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("queue first: %v", err)
	}
	// A compatible message must fold into the tail instead of being rejected:
	// the fold is the admission, so the queue never reports full for a message
	// that would merge anyway.
	second, err := svc.QueueMessage(context.Background(), "session", "task", "second", "", QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("compatible admission at capacity = %v, want fold", err)
	}
	if second.ID != first.ID || second.Content != "first\n\nsecond" {
		t.Fatalf("second admission = %+v, want folded survivor %s", second, first.ID)
	}
	status := svc.GetStatus(context.Background(), "session")
	if status.Count != 1 || status.Entries[0].ID != first.ID || status.Entries[0].Content != "first\n\nsecond" {
		t.Fatalf("queue after full-queue fold: %+v", status)
	}
}

func TestService_AutoMergeAtFullQueueRejectsIncompatibleAdmission(t *testing.T) {
	svc := newAutoMergeTestService(t, 1)
	if _, err := svc.QueueMessage(context.Background(), "session", "task", "first", "model-a", QueuedByUser, false, nil); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	second, err := svc.QueueMessage(context.Background(), "session", "task", "second", "model-b", QueuedByUser, false, nil)
	if !errors.Is(err, ErrQueueFull) || second != nil {
		t.Fatalf("incompatible full admission = %+v, err=%v, want queue full", second, err)
	}
}

func TestService_AutoMergeDisabledAtFullQueueRejects(t *testing.T) {
	svc := newAutoMergeTestService(t, 1)
	svc.SetAutoMergeEnabled(false)
	if _, err := svc.QueueMessage(context.Background(), "session", "task", "first", "", QueuedByUser, false, nil); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	second, err := svc.QueueMessage(context.Background(), "session", "task", "second", "", QueuedByUser, false, nil)
	if !errors.Is(err, ErrQueueFull) || second != nil {
		t.Fatalf("disabled full admission = %+v, err=%v, want queue full", second, err)
	}
}

func TestService_AutoMergeAtFullQueueSkipsAfterInsertHook(t *testing.T) {
	svc := newAutoMergeTestService(t, 1)
	if _, err := svc.QueueMessage(context.Background(), "session", "task", "first", "", QueuedByUser, false, nil); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	ran := false
	second, err := svc.QueueMessageWithMetadataAfterInsert(
		context.Background(), "session", "task", "second", "", QueuedByUser, false, nil, nil,
		func(context.Context, *QueuedMessage) error { ran = true; return nil },
	)
	if !errors.Is(err, ErrQueueFull) || second != nil {
		t.Fatalf("after-insert full admission = %+v, err=%v, want queue full", second, err)
	}
	if ran {
		t.Fatal("after-insert hook ran for a rejected full-queue admission")
	}
}

// autoMergeCandidateErrorRepository wraps a repository and fails every
// full-queue candidate fold with the configured error.
type autoMergeCandidateErrorRepository struct {
	Repository
	err error
}

func (r *autoMergeCandidateErrorRepository) AutoMergeCandidateIntoAbove(context.Context, *QueuedMessage) (*QueuedMessage, bool, error) {
	return nil, false, r.err
}

// failNextInsertRepository fails the next N Insert calls with ErrQueueFull,
// simulating a cross-process admission that observed a stale full count while
// the underlying queue already drained.
type failNextInsertRepository struct {
	Repository
	mu       sync.Mutex
	failNext int
}

func (r *failNextInsertRepository) Insert(ctx context.Context, msg *QueuedMessage, maxPerSession int) error {
	r.mu.Lock()
	if r.failNext > 0 {
		r.failNext--
		r.mu.Unlock()
		return ErrQueueFull
	}
	r.mu.Unlock()
	return r.Repository.Insert(ctx, msg, maxPerSession)
}

func TestService_AutoMergeFullQueueRetriesInsertAfterFoldSkip(t *testing.T) {
	repo := &failNextInsertRepository{Repository: NewMemoryRepository()}
	svc := newAutoMergeTestServiceWithRepository(t, repo, 1)
	if _, err := svc.QueueMessage(context.Background(), "session", "task", "first", "", QueuedByUser, false, nil); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	// A concurrent drain frees capacity between the failed insert and the
	// fold scan: the fold sees an empty queue and skips, and admission must
	// retry the ordinary insert instead of returning the stale ErrQueueFull.
	if _, ok := svc.TakeQueued(context.Background(), "session"); !ok {
		t.Fatal("drain first entry")
	}
	repo.mu.Lock()
	repo.failNext = 1
	repo.mu.Unlock()

	second, err := svc.QueueMessage(context.Background(), "session", "task", "second", "", QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("admission after fold skip = %v, want insert retry to succeed", err)
	}
	if second == nil || second.Content != "second" {
		t.Fatalf("queued entry = %+v, want the retried message", second)
	}
	status := svc.GetStatus(context.Background(), "session")
	if status.Count != 1 || status.Entries[0].Content != "second" {
		t.Fatalf("status = %+v, want the retried message queued", status)
	}
}

func TestService_AutoMergeFullQueueCandidateMergeErrorDegradesToQueueFull(t *testing.T) {
	wantErr := errors.New("automatic candidate merge unavailable")
	repo := &autoMergeCandidateErrorRepository{Repository: NewMemoryRepository(), err: wantErr}
	svc := newAutoMergeTestServiceWithRepository(t, repo, 1)
	first, err := svc.QueueMessage(context.Background(), "session", "task", "first", "", QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("queue first: %v", err)
	}
	// The full-queue fold itself failed: admission must degrade to the
	// original ErrQueueFull (never accept a message whose fold did not
	// happen, and never surface the repository error).
	second, err := svc.QueueMessage(context.Background(), "session", "task", "second", "", QueuedByUser, false, nil)
	if !errors.Is(err, ErrQueueFull) || second != nil {
		t.Fatalf("candidate merge error admission = %+v, err=%v, want queue full", second, err)
	}
	status := svc.GetStatus(context.Background(), "session")
	if status.Count != 1 || status.Entries[0].ID != first.ID || status.Entries[0].Content != "first" {
		t.Fatalf("queue changed after degraded full admission: %+v", status)
	}
}

func TestService_AutoMergeFullQueueTaskInactiveSurfacesContract(t *testing.T) {
	repo := &autoMergeCandidateErrorRepository{Repository: NewMemoryRepository(), err: ErrTaskInactive}
	svc := newAutoMergeTestServiceWithRepository(t, repo, 1)
	if _, err := svc.QueueMessage(context.Background(), "session", "task", "first", "", QueuedByUser, false, nil); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	// The full-queue fold's task guard failed (task archived/deleted between
	// the failed insert and the fold): admission must surface ErrTaskInactive,
	// not the stale ErrQueueFull.
	second, err := svc.QueueMessage(context.Background(), "session", "task", "second", "", QueuedByUser, false, nil)
	if !errors.Is(err, ErrTaskInactive) || second != nil {
		t.Fatalf("task-inactive admission = %+v, err=%v, want ErrTaskInactive", second, err)
	}
	status := svc.GetStatus(context.Background(), "session")
	if status.Count != 1 || status.Entries[0].Content != "first" {
		t.Fatalf("queue changed after rejected admission: %+v", status)
	}
}

func TestService_AutoMergeStorageErrorDegradesToSeparateAdmission(t *testing.T) {
	wantErr := errors.New("automatic merge unavailable")
	repo := &autoMergeErrorRepository{Repository: NewMemoryRepository(), err: wantErr}
	svc := newAutoMergeTestServiceWithRepository(t, repo, 10)
	first, err := svc.QueueMessage(context.Background(), "session", "task", "first", "", QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("queue first: %v", err)
	}
	second, err := svc.QueueMessage(context.Background(), "session", "task", "second", "", QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("post-insert merge error escaped admission: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("failed fold should keep distinct source identity")
	}
	if status := svc.GetStatus(context.Background(), "session"); status.Count != 2 {
		t.Fatalf("degraded queue count = %d, want 2", status.Count)
	}
}

func TestService_AutoMergeSerializesDeferredFinalizationAndLaterAdmission(t *testing.T) {
	svc := newAutoMergeTestService(t, 10)
	inserted := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := svc.QueueMessageWithMetadataAfterInsert(
			context.Background(), "session", "task", "first", "", QueuedByUser, false, nil, nil,
			func(context.Context, *QueuedMessage) error {
				close(inserted)
				<-release
				return nil
			},
		)
		firstDone <- err
	}()
	<-inserted

	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		_, err := svc.QueueMessage(context.Background(), "session", "task", "second", "", QueuedByUser, false, nil)
		secondDone <- err
	}()
	<-secondStarted
	select {
	case err := <-secondDone:
		t.Fatalf("later admission bypassed provisional source lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first admission: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second admission: %v", err)
	}
	status := svc.GetStatus(context.Background(), "session")
	if status.Count != 1 || status.Entries[0].Content != "first\n\nsecond" {
		t.Fatalf("serialized queue = %+v", status)
	}
}

func TestService_AutoMergeSerializesFinalizationWithDrainAndRemoval(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*testing.T, *Service, *QueuedMessage, chan struct{})
	}{
		{
			name: "drain",
			run: func(t *testing.T, svc *Service, source *QueuedMessage, release chan struct{}) {
				started := make(chan struct{})
				result := make(chan struct {
					msg *QueuedMessage
					ok  bool
				}, 1)
				go func() {
					close(started)
					msg, ok := svc.ReserveQueued(context.Background(), source.SessionID)
					result <- struct {
						msg *QueuedMessage
						ok  bool
					}{msg: msg, ok: ok}
				}()
				<-started
				select {
				case got := <-result:
					t.Fatalf("drain bypassed admission finalization: %+v", got)
				case <-time.After(50 * time.Millisecond):
				}
				close(release)
				got := <-result
				if !got.ok || got.msg == nil || got.msg.ID != source.ID {
					t.Fatalf("drain result = %+v, want source %s", got, source.ID)
				}
			},
		},
		{
			name: "remove",
			run: func(t *testing.T, svc *Service, source *QueuedMessage, release chan struct{}) {
				started := make(chan struct{})
				result := make(chan error, 1)
				go func() {
					close(started)
					result <- svc.RemoveEntry(context.Background(), source.SessionID, source.ID)
				}()
				<-started
				select {
				case err := <-result:
					t.Fatalf("removal bypassed admission finalization: %v", err)
				case <-time.After(50 * time.Millisecond):
				}
				close(release)
				if err := <-result; err != nil {
					t.Fatalf("remove after finalization: %v", err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := newAutoMergeTestService(t, 10)
			release := make(chan struct{})
			inserted := make(chan *QueuedMessage, 1)
			admissionDone := make(chan error, 1)
			go func() {
				_, err := svc.QueueMessageWithMetadataAfterInsert(
					context.Background(), "session", "task", "first", "", QueuedByUser, false, nil, nil,
					func(_ context.Context, source *QueuedMessage) error {
						inserted <- source
						<-release
						return nil
					},
				)
				admissionDone <- err
			}()
			source := <-inserted
			test.run(t, svc, source, release)
			if err := <-admissionDone; err != nil {
				t.Fatalf("admission: %v", err)
			}
		})
	}
}

func TestService_AutoMergeSkipsExplicitMutationContracts(t *testing.T) {
	t.Run("coalesce insert", func(t *testing.T) {
		svc := newAutoMergeTestService(t, 10)
		for _, key := range []string{"one", "two"} {
			if _, _, err := svc.QueueMessageWithCoalesceKey(
				context.Background(), "session", "task", key, "", QueuedByUser, false, nil, nil, key, true,
			); err != nil {
				t.Fatalf("queue coalesced %s: %v", key, err)
			}
		}
		if got := svc.GetStatus(context.Background(), "session").Count; got != 2 {
			t.Fatalf("coalesce entries auto-merged: count=%d", got)
		}
	})

	t.Run("restore", func(t *testing.T) {
		svc := newAutoMergeTestService(t, 10)
		svc.SetAutoMergeEnabled(false)
		first, _ := svc.QueueMessage(context.Background(), "session", "task", "first", "", QueuedByUser, false, nil)
		_, _ = svc.QueueMessage(context.Background(), "session", "task", "second", "", QueuedByUser, false, nil)
		_, _, _ = svc.TakeQueuedEntry(context.Background(), "session", first.ID)
		svc.SetAutoMergeEnabled(true)
		if _, err := svc.RestoreMessage(context.Background(), first); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if got := svc.GetStatus(context.Background(), "session").Count; got != 2 {
			t.Fatalf("restore auto-merged: count=%d", got)
		}
	})

	t.Run("retry", func(t *testing.T) {
		svc := newAutoMergeTestService(t, 10)
		svc.SetAutoMergeEnabled(false)
		first, _ := svc.QueueMessage(context.Background(), "session", "task", "first", "", QueuedByUser, false, nil)
		_, _ = svc.QueueMessage(context.Background(), "session", "task", "second", "", QueuedByUser, false, nil)
		_, _, _ = svc.TakeQueuedEntry(context.Background(), "session", first.ID)
		svc.SetAutoMergeEnabled(true)
		if _, _, err := svc.RequeueMessage(context.Background(), first, QueuedByUser, ""); err != nil {
			t.Fatalf("requeue: %v", err)
		}
		if got := svc.GetStatus(context.Background(), "session").Count; got != 2 {
			t.Fatalf("retry auto-merged: count=%d", got)
		}
	})
}

type autoMergeErrorRepository struct {
	Repository
	err error
}

func (r *autoMergeErrorRepository) AutoMergeIntoAbove(context.Context, string, string) (*QueuedMessage, bool, error) {
	return nil, false, r.err
}

func newAutoMergeTestService(t *testing.T, max int) *Service {
	t.Helper()
	return newAutoMergeTestServiceWithRepository(t, NewMemoryRepository(), max)
}

func newAutoMergeTestServiceWithRepository(t *testing.T, repo Repository, max int) *Service {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	return NewService(repo, max, log)
}

func TestService_AutoMergeDefaultsOnAndFoldsCompatibleAdmissions(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	svc := NewServiceMemory(log)
	settings, ok := interface{}(svc).(automaticMergeSettings)
	if !ok {
		t.Fatalf("service %T does not expose an independent automatic-merge setting", svc)
	}
	if !settings.AutoMergeEnabled() {
		t.Fatal("automatic merge should default on")
	}

	first, err := svc.QueueMessage(context.Background(), "session", "task", "first", "model", QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("queue first: %v", err)
	}
	second, err := svc.QueueMessage(context.Background(), "session", "task", "second", "model", QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("queue second: %v", err)
	}
	if second.ID != first.ID || second.Content != "first\n\nsecond" {
		t.Fatalf("second admission = %+v, want surviving first entry", second)
	}
	status := svc.GetStatus(context.Background(), "session")
	if status.Count != 1 || status.Entries[0].ID != first.ID {
		t.Fatalf("status = %+v, want one merged entry", status)
	}
}
