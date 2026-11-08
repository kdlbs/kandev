package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

type testInferenceCapacityOwner interface {
	SetInferenceCapacityOwner(*sessioncapacity.Store, func(context.Context))
	ReserveInferenceCapacity(context.Context, sessioncapacity.InferenceBinding) (bool, string, error)
	SyncInferenceCapacity(context.Context, bool, func(map[string]sessioncapacity.InferenceBinding) error) error
	InferenceCapacityBindings(context.Context) (map[string]sessioncapacity.InferenceBinding, error)
	InferenceCapacityChanged() <-chan struct{}
}

func newInferenceCapacityStore(t *testing.T) *sessioncapacity.Store {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	raw, err := systemsettings.NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatal(err)
	}
	return sessioncapacity.NewStore(raw)
}

func attachInferenceCapacity(t *testing.T, svc *Service, known bool) testInferenceCapacityOwner {
	t.Helper()
	target, ok := any(svc).(testInferenceCapacityOwner)
	if !ok {
		t.Fatal("actual Service lacks common inference owner entry")
	}
	target.SetInferenceCapacityOwner(newInferenceCapacityStore(t), nil)
	if err := target.SyncInferenceCapacity(context.Background(), known, func(map[string]sessioncapacity.InferenceBinding) error { return nil }); err != nil {
		t.Fatal(err)
	}
	return target
}

func newCommonInferenceBinding(attempt, grade string) sessioncapacity.InferenceBinding {
	return sessioncapacity.InferenceBinding{Key: "inference:owner:" + attempt, InstanceID: "instance", OwnerID: "owner", AttemptID: attempt, OwnerKind: "utility", Grade: grade, GradeKnown: true, State: "reserved"}
}

func TestInferenceCapacityTaskAndChildUseOneANDPopulation(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithCeiling(t, 1)
	svc.sessionCeiling.lister = repo
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
	applyGradeCapacity(t, svc, 1, 1)
	owner := attachInferenceCapacity(t, svc, true)
	binding := newCommonInferenceBinding("child", "A")
	admitted, reason, err := owner.ReserveInferenceCapacity(ctx, binding)
	if err != nil || !admitted || reason != "" {
		t.Fatalf("child admission=%t %s %v", admitted, reason, err)
	}
	createGradeOwner(t, repo, "incoming", "A", false)
	decision := svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "incoming", sessionID: "incoming-session", origin: launchOriginManual})
	if decision.admitted || decision.reasonCode != ceilingReasonBothLimits || decision.population != 1 {
		t.Fatalf("child omitted or counted twice: %+v", decision)
	}
	stored, err := owner.InferenceCapacityBindings(ctx)
	if err != nil || len(stored) != 1 || stored[binding.Key].Grade != "A" {
		t.Fatalf("owner not durable: %+v %v", stored, err)
	}
}

func TestInferenceCapacityLastSeatHasOneTaskOrChildWinner(t *testing.T) {
	for range 4 {
		ctx := context.Background()
		svc, repo := newServiceWithCeiling(t, 1)
		svc.sessionCeiling.lister = repo
		svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
		applyGradeCapacity(t, svc, 1, 1)
		owner := attachInferenceCapacity(t, svc, true)
		createGradeOwner(t, repo, "incoming", "A", false)
		start := make(chan struct{})
		results := make(chan bool, 2)
		go func() {
			<-start
			results <- svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "incoming", sessionID: "incoming-session", origin: launchOriginAutomatic}).admitted
		}()
		go func() {
			<-start
			admitted, _, err := owner.ReserveInferenceCapacity(ctx, newCommonInferenceBinding("child", "A"))
			if err != nil {
				t.Error(err)
			}
			results <- admitted
		}()
		close(start)
		a, b := <-results, <-results
		if a == b {
			t.Fatalf("last seat winners: %t %t", a, b)
		}
		stored, err := owner.InferenceCapacityBindings(ctx)
		if err != nil || len(stored)+len(svc.sessionCeiling.reservations) != 1 {
			t.Fatalf("owner population=%d+%d err=%v", len(stored), len(svc.sessionCeiling.reservations), err)
		}
	}
}

func TestInferenceCapacityMissingInventoryIsNotAnEmptySeat(t *testing.T) {
	for _, known := range []bool{false, true} {
		t.Run(fmt.Sprint(known), func(t *testing.T) {
			ctx := context.Background()
			svc, repo := newServiceWithCeiling(t, 1)
			svc.sessionCeiling.lister = repo
			svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
			applyGradeCapacity(t, svc, 1, 1)
			attachInferenceCapacity(t, svc, known)
			createGradeOwner(t, repo, "incoming", "A", false)
			d := svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "incoming", sessionID: "incoming-session", origin: launchOriginManual})
			if d.admitted != known {
				t.Fatalf("known=%t decision=%+v", known, d)
			}
			if !known && d.reasonCode != ceilingReasonUnknownPopulation {
				t.Fatalf("unknown became free: %+v", d)
			}
		})
	}
}

func TestInferenceCapacityHeldIdentityIsFrozenAndForeignInstanceRejected(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithCeiling(t, 1)
	svc.sessionCeiling.lister = repo
	applyGradeCapacity(t, svc, 1, 1)
	owner := attachInferenceCapacity(t, svc, true)
	binding := newCommonInferenceBinding("child", "A")
	if ok, _, err := owner.ReserveInferenceCapacity(ctx, binding); err != nil || !ok {
		t.Fatalf("initial=%t %v", ok, err)
	}
	applyGradeCapacity(t, svc, 1, 0)
	repeated := binding
	repeated.Grade = "B"
	if ok, _, err := owner.ReserveInferenceCapacity(ctx, repeated); err != nil || !ok {
		t.Fatalf("existing owner rejected after limit change: %t %v", ok, err)
	}
	foreign := binding
	foreign.InstanceID = "replacement-instance"
	if ok, _, err := owner.ReserveInferenceCapacity(ctx, foreign); err == nil || ok {
		t.Fatalf("foreign generation admitted: %t %v", ok, err)
	}
	stored, err := owner.InferenceCapacityBindings(ctx)
	if err != nil || len(stored) != 1 || stored[binding.Key] != binding {
		t.Fatalf("old owner replaced: %+v %v", stored, err)
	}
}

func TestInferenceCapacityAutomaticKeepsLegacyAdmission(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithCeiling(t, 1)
	svc.sessionCeiling.lister = repo
	owner := attachInferenceCapacity(t, svc, false)
	if ok, _, err := owner.ReserveInferenceCapacity(ctx, newCommonInferenceBinding("legacy-child", "")); err != nil || !ok {
		t.Fatalf("legacy child=%t %v", ok, err)
	}
	createGradeOwner(t, repo, "incoming", "", false)
	if d := svc.sessionCeiling.admit(ctx, admissionRequest{sessionID: "incoming-session", origin: launchOriginAutomatic}); !d.admitted {
		t.Fatalf("legacy policy changed: %+v", d)
	}
}

func TestInferenceCapacityObservationUsesAdmissionPopulation(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithCeiling(t, 2)
	svc.sessionCeiling.lister = repo
	applyGradeCapacity(t, svc, 2, 2)
	owner := attachInferenceCapacity(t, svc, true)
	if ok, _, err := owner.ReserveInferenceCapacity(ctx, newCommonInferenceBinding("child", "A")); err != nil || !ok {
		t.Fatalf("child=%t %v", ok, err)
	}
	observed, err := svc.CurrentSessionCeilingObservation(ctx)
	if err != nil || !observed.Known || observed.InUse != 1 {
		t.Fatalf("observation omitted admitted child: %+v %v", observed, err)
	}
	if err := owner.SyncInferenceCapacity(ctx, false, func(map[string]sessioncapacity.InferenceBinding) error { return nil }); err != nil {
		t.Fatal(err)
	}
	observed, err = svc.CurrentSessionCeilingObservation(ctx)
	if err == nil || observed.Known {
		t.Fatalf("unknown inventory advertised free count: %+v %v", observed, err)
	}
	stored, err := owner.InferenceCapacityBindings(ctx)
	if err != nil || len(stored) != 1 {
		t.Fatalf("unknown observation erased child: %+v %v", stored, err)
	}
}

func TestInferenceCapacityRequestedAndHeldClassificationReasons(t *testing.T) {
	for _, tc := range []struct {
		name, grade, want string
		known             bool
	}{
		{"configured", "A", ceilingReasonBothLimits, true},
		{"unconfigured", "", ceilingReasonNeedsConfiguration, true},
		{"missing_limit", "Other", ceilingReasonNeedsConfiguration, true},
		{"unknown", "A", ceilingReasonUnknownPopulation, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc, repo := newServiceWithCeiling(t, 1)
			svc.sessionCeiling.lister = repo
			applyGradeCapacity(t, svc, 1, 1)
			owner := attachInferenceCapacity(t, svc, true)
			prior := newCommonInferenceBinding("prior", tc.grade)
			prior.GradeKnown = tc.known
			if err := owner.SyncInferenceCapacity(ctx, true, func(active map[string]sessioncapacity.InferenceBinding) error { active[prior.Key] = prior; return nil }); err != nil {
				t.Fatal(err)
			}
			ok, reason, err := owner.ReserveInferenceCapacity(ctx, newCommonInferenceBinding("next", "A"))
			if ok || err != nil || reason != tc.want {
				t.Fatalf("held classification ignored: %t %s %v", ok, reason, err)
			}
			stored, err := owner.InferenceCapacityBindings(ctx)
			if err != nil || len(stored) != 1 || stored[prior.Key] != prior {
				t.Fatalf("refusal changed owner: %+v %v", stored, err)
			}
		})
	}
}
