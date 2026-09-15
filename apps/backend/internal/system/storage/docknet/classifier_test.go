package docknet

import (
	"context"
	"errors"
	"testing"
	"time"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
)

type fakeOracle struct {
	key     string
	lookup  TaskLookup
	err     error
	lastNet agentdocker.NetworkInfo
	lastCtx context.Context
}

type contextKey struct{}

func (o *fakeOracle) Ownership(ctx context.Context, network agentdocker.NetworkInfo) (string, TaskLookup, error) {
	o.lastCtx = ctx
	o.lastNet = network
	return o.key, o.lookup, o.err
}

func TestClassifyPassesContextToTaskOracle(t *testing.T) {
	oracle := &fakeOracle{key: activeTaskKey, lookup: TaskLookupActive}
	ctx := context.WithValue(context.Background(), contextKey{}, "maintenance-lease")
	Classify(ctx, taskNetwork("kd_context", map[string]string{"kandev.task_id": activeTaskKey}), oracle,
		time.Time{}, graceWindow, staleAge, classifyOptions)
	if oracle.lastCtx != ctx {
		t.Fatal("Classify must pass the maintenance context to the task oracle")
	}
}

var classifyOptions = ClassifyOptions{Now: func() time.Time {
	return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
}}

const (
	activeTaskKey = "task-active-1"
	staleAge      = 168 * time.Hour
	graceWindow   = time.Hour
)

func taskNetwork(name string, labels map[string]string) agentdocker.NetworkInfo {
	if labels == nil {
		labels = map[string]string{}
	}
	return agentdocker.NetworkInfo{
		ID: "net-" + name, Name: name, Driver: "bridge", Scope: "local", Labels: labels,
	}
}

func TestClassifyActiveNetworkNeverRemoved(t *testing.T) {
	// AC1: connected container + active owning task => active, never removed.
	network := taskNetwork("kd_a_default", map[string]string{"kandev.task_id": activeTaskKey})
	network.Containers = map[string]string{"container-1": "web"}
	got := Classify(context.Background(), network, &fakeOracle{key: activeTaskKey, lookup: TaskLookupActive},
		time.Time{}, graceWindow, staleAge, classifyOptions)

	if got.Class != ClassActive {
		t.Fatalf("class = %s, want active", got.Class)
	}
	if Eligible(got.Class) {
		t.Fatal("active network must never be eligible")
	}
	if got.Evidence.ConnectedContainers != 1 {
		t.Fatalf("evidence containers = %d, want 1", got.Evidence.ConnectedContainers)
	}
}

func TestClassifyAttachedOwnedByActiveTaskWithoutContainers(t *testing.T) {
	// AC2: kandev labels, owning task active, zero containers => attached.
	network := taskNetwork("kd_b", map[string]string{"kandev.task_id": activeTaskKey})
	got := Classify(context.Background(), network, &fakeOracle{key: activeTaskKey, lookup: TaskLookupActive},
		time.Time{}, graceWindow, staleAge, classifyOptions)

	if got.Class != ClassAttached {
		t.Fatalf("class = %s, want attached", got.Class)
	}
	if Eligible(got.Class) {
		t.Fatal("attached network must never be eligible")
	}
}

func TestClassifyOrphanedAfterGraceWindow(t *testing.T) {
	// AC3: owner archived/terminal, zero containers, grace elapsed => orphaned.
	network := taskNetwork("kd_c", map[string]string{"kandev.task_id": "task-gone"})
	firstSeen := classifyOptions.Now().Add(-2 * graceWindow)
	got := Classify(
		context.Background(), network, &fakeOracle{key: "task-gone", lookup: TaskLookupInactive},
		firstSeen, graceWindow, staleAge, classifyOptions,
	)

	if got.Class != ClassOrphaned {
		t.Fatalf("class = %s, want orphaned", got.Class)
	}
	if !Eligible(got.Class) {
		t.Fatal("orphaned past grace must be eligible")
	}
	if !got.Evidence.PastGraceWindow {
		t.Fatal("evidence must record the elapsed grace window")
	}
}

func TestClassifyOrphanedInsideGraceWindowKept(t *testing.T) {
	network := taskNetwork("kd_c", map[string]string{"kandev.task_id": "task-gone"})
	firstSeen := classifyOptions.Now().Add(-graceWindow / 2)
	got := Classify(
		context.Background(), network, &fakeOracle{key: "task-gone", lookup: TaskLookupInactive},
		firstSeen, graceWindow, staleAge, classifyOptions,
	)

	if got.Class != ClassStaleUncertain || Eligible(got.Class) {
		t.Fatalf("class = %s, want stale_uncertain while grace runs", got.Class)
	}
}

func TestClassifyOracleErrorFailsClosed(t *testing.T) {
	// AC3 fail-closed: any task-repository read error => stale-uncertain, skip.
	network := taskNetwork("kd_d", map[string]string{"kandev.task_id": "task-err"})
	got := Classify(
		context.Background(), network, &fakeOracle{err: errors.New("task store unavailable")},
		time.Time{}, graceWindow, staleAge, classifyOptions,
	)

	if got.Class != ClassStaleUncertain {
		t.Fatalf("class = %s, want stale_uncertain on oracle error", got.Class)
	}
	if Eligible(got.Class) {
		t.Fatal("oracle-error network must never be eligible")
	}
}

func TestClassifySafelyStaleAfterStableAge(t *testing.T) {
	// AC4: unresolvable owner + first-seen >= staleAge => safely_stale.
	network := taskNetwork("kd_e", map[string]string{"com.docker.compose.project": "kd_unknown"})
	firstSeen := classifyOptions.Now().Add(-staleAge)
	got := Classify(
		context.Background(), network, &fakeOracle{key: "kd_unknown", lookup: TaskLookupUnknown},
		firstSeen, graceWindow, staleAge, classifyOptions,
	)

	if got.Class != ClassSafelyStale || !Eligible(got.Class) {
		t.Fatalf("class = %s, want safely_stale eligible", got.Class)
	}
	if !got.Evidence.FirstSeenStable {
		t.Fatal("evidence must record the stable first-seen age")
	}
}

func TestClassifySafelyStaleFirstSightingKept(t *testing.T) {
	// AC4: first sighting < threshold => keep observing.
	network := taskNetwork("kd_f", nil)
	got := Classify(
		context.Background(), network, &fakeOracle{key: "", lookup: TaskLookupUnknown},
		classifyOptions.Now(), graceWindow, staleAge, classifyOptions,
	)

	if got.Class != ClassStaleUncertain || Eligible(got.Class) {
		t.Fatalf("class = %s, want stale_uncertain on first sighting", got.Class)
	}
}

func TestClassifyExcludesSpecialNetworks(t *testing.T) {
	// AC5: bridge/host/none names and non-bridge drivers always excluded.
	tests := []struct {
		name string
		net  agentdocker.NetworkInfo
	}{
		{name: "default bridge", net: taskNetwork("bridge", nil)},
		{name: "host", net: taskNetwork("host", nil)},
		{name: "none", net: taskNetwork("none", nil)},
		{
			name: "overlay driver",
			net:  agentdocker.NetworkInfo{ID: "ov", Name: "kd_overlay", Driver: "overlay"},
		},
		{
			name: "no driver",
			net:  agentdocker.NetworkInfo{ID: "nd", Name: "weird"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(context.Background(), tt.net, &fakeOracle{}, time.Time{}, graceWindow, staleAge, classifyOptions)
			if got.Class != ClassExcluded && got.Class != ClassStaleUncertain {
				t.Fatalf("class = %s, want excluded", got.Class)
			}
			if got.Class == ClassStaleUncertain {
				// A missing driver is a structural exclusion; oracle errors on
				// excluded networks must still fail closed to excluded here.
				t.Fatalf("class = %s, want excluded", got.Class)
			}
		})
	}
}

func TestClassifyOracleNotConsultedForExcluded(t *testing.T) {
	bridge := taskNetwork("bridge", nil)
	oracle := &fakeOracle{err: errors.New("must not be consulted")}
	got := Classify(context.Background(), bridge, oracle, time.Time{}, graceWindow, staleAge, classifyOptions)
	if got.Class != ClassExcluded {
		t.Fatalf("class = %s, want excluded", got.Class)
	}
	if oracle.lastNet.ID != "" {
		t.Fatal("oracle must not be consulted for excluded networks")
	}
}

func TestOwnershipKeyPrefersKandevTaskID(t *testing.T) {
	// Both label families are honored; kandev wins when both exist.
	if key := OwnershipKeyFromLabels(map[string]string{
		"kandev.task_id": "task-1", "com.docker.compose.project": "kd_x",
	}); key != "task-1" {
		t.Fatalf("key = %q, want kandev task id", key)
	}
	if key := OwnershipKeyFromLabels(map[string]string{
		"com.docker.compose.project": "kd_x",
	}); key != "kd_x" {
		t.Fatalf("key = %q, want compose project", key)
	}
	if key := OwnershipKeyFromLabels(map[string]string{"other": "x"}); key != "" {
		t.Fatalf("key = %q, want empty for unrelated labels", key)
	}
}
