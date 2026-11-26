package coordinator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

func TestChangeViewNoChecksAndUnknownNeverPassing(t *testing.T) {
	e, r := newObserveEnv(t)
	r.snap = mergestate.Snapshot{Key: testKey, State: mergestate.StateOpen, HeadSHA: "h1", TargetBranch: "main",
		Pipeline: mergestate.PipelineNone, UnresolvedThreads: -1, Behind: -1}
	vs, err := e.svc.ChangeViews(context.Background(), "ws-1", "tm", true)
	if err != nil || len(vs) != 1 {
		t.Fatalf("views = %v err = %v", vs, err)
	}
	v := vs[0]
	if v.Pipeline != "none" || v.Mergeable != ReadingUnavailable || v.Approvals != ReadingUnavailable ||
		v.Conflict != ReadingUnavailable || v.UnresolvedThreads != nil || v.Behind != nil || v.Unavailable {
		t.Fatalf("view = %+v", v)
	}
}

func TestChangeViewFailedReadKeepsLastGood(t *testing.T) {
	e, r := newObserveEnv(t)
	r.snap = mergestate.Snapshot{Key: testKey, State: mergestate.StateOpen, HeadSHA: "h1", Pipeline: mergestate.PipelineGreen,
		Conflict: mergestate.ConflictClear, UnresolvedThreads: 2, Behind: 0}
	ok, _ := e.svc.ChangeViews(context.Background(), "ws-1", "tm", true)
	r.err = errors.New("rate limited")
	got, err := e.svc.ChangeViews(context.Background(), "ws-1", "tm", true)
	if err != nil || len(got) != 1 {
		t.Fatalf("views = %v err = %v", got, err)
	}
	v := got[0]
	if !v.Unavailable || v.Pipeline != "green" || *v.UnresolvedThreads != 2 || !v.ObservedAt.Equal(ok[0].ObservedAt) || v.ObservedAt.IsZero() {
		t.Fatalf("last good not kept: %+v", v)
	}
}

func TestChangeViewNeverReadIsUnavailable(t *testing.T) {
	e, r := newObserveEnv(t)
	r.err = errors.New("down")
	vs, _ := e.svc.ChangeViews(context.Background(), "ws-1", "tm", false)
	if len(vs) != 1 || !vs[0].Unavailable || vs[0].Pipeline != "unavailable" || !vs[0].ObservedAt.IsZero() {
		t.Fatalf("views = %+v", vs)
	}
}

func TestChangeViewNoLinksOrFlagOffIsEmpty(t *testing.T) {
	e, _ := newObserveEnv(t)
	vs, _ := e.svc.ChangeViews(context.Background(), "ws-1", "nobody", false)
	if len(vs) != 0 {
		t.Fatalf("views = %v", vs)
	}
	e.svc.merge = false
	vs, _ = e.svc.ChangeViews(context.Background(), "ws-1", "tm", false)
	if vs != nil {
		t.Fatalf("flag off views = %v", vs)
	}
}
