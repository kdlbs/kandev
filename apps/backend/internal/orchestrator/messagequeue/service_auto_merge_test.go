package messagequeue

import (
	"context"
	"errors"
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

func TestService_AutoMergeDoesNotBypassCapacity(t *testing.T) {
	svc := newAutoMergeTestService(t, 1)
	first, err := svc.QueueMessage(context.Background(), "session", "task", "first", "", QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("queue first: %v", err)
	}
	second, err := svc.QueueMessage(context.Background(), "session", "task", "second", "", QueuedByUser, false, nil)
	if !errors.Is(err, ErrQueueFull) || second != nil {
		t.Fatalf("second admission = %+v, err=%v, want queue full", second, err)
	}
	status := svc.GetStatus(context.Background(), "session")
	if status.Count != 1 || status.Entries[0].ID != first.ID || status.Entries[0].Content != "first" {
		t.Fatalf("queue changed after full admission: %+v", status)
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
