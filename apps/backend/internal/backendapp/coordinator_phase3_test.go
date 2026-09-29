package backendapp

import (
	"context"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestPhase3Effective_EightCombinations(t *testing.T) {
	for _, on := range []bool{false, true} {
		for _, p2 := range []bool{false, true} {
			for _, p3 := range []bool{false, true} {
				want := on && p2 && p3
				if got := phase3Effective(on, p2, p3); got != want {
					t.Fatalf("phase3Effective(%v,%v,%v) = %v, want %v", on, p2, p3, got, want)
				}
			}
		}
	}
}

func TestInitCoordinatorWiring_Phase3FollowsAllThreeFlags(t *testing.T) {
	for _, on := range []bool{false, true} {
		for _, p2 := range []bool{false, true} {
			for _, p3 := range []bool{false, true} {
				svc, err := initCoordinatorWiring(context.Background(), newCoordinatorTestPool(t), newCoordinatorTestTracker(t), nil, nil, nil, on, p2, p3, newTestLogger())
				if err != nil {
					t.Fatalf("initCoordinatorWiring: %v", err)
				}
				if svc == nil {
					continue
				}
				if want := on && p2 && p3; svc.Phase3Enabled() != want {
					t.Fatalf("enabled=%v p2=%v p3=%v: Phase3Enabled = %v", on, p2, p3, svc.Phase3Enabled())
				}
			}
		}
	}
}

func TestRegisterCoordinatorRoutes_Phase3RegistrationsOnlyWhenEffective(t *testing.T) {
	gin.SetMode(gin.TestMode)
	names := []string{"containment", "spend", "wake", "delivery", "relay", "reply", "automatic", "improvements"}
	for _, p2 := range []bool{false, true} {
		for _, p3 := range []bool{false, true} {
			svc, err := initCoordinatorWiring(context.Background(), newCoordinatorTestPool(t), newCoordinatorTestTracker(t), nil, nil, nil, true, p2, p3, newTestLogger())
			if err != nil {
				t.Fatalf("initCoordinatorWiring: %v", err)
			}
			called := map[string]int{}
			mk := func(name string) coordinatorRegistration {
				return func(*gin.Engine, bus.EventBus, *coordinator.Service, *logger.Logger) func(context.Context, time.Time) {
					called[name]++
					return func(context.Context, time.Time) {}
				}
			}
			saved := []coordinatorRegistration{registerCoordinatorContainment, registerCoordinatorSpend, registerCoordinatorWake, registerCoordinatorDelivery, registerCoordinatorRelay, registerCoordinatorReply, registerCoordinatorAutomatic, registerCoordinatorImprovements}
			registerCoordinatorContainment, registerCoordinatorSpend = mk(names[0]), mk(names[1])
			registerCoordinatorWake, registerCoordinatorDelivery = mk(names[2]), mk(names[3])
			registerCoordinatorRelay, registerCoordinatorReply = mk(names[4]), mk(names[5])
			registerCoordinatorAutomatic, registerCoordinatorImprovements = mk(names[6]), mk(names[7])
			origPass := runCoordinatorBackgroundPass
			t.Cleanup(func() {
				runCoordinatorBackgroundPass = origPass
				registerCoordinatorContainment, registerCoordinatorSpend, registerCoordinatorWake, registerCoordinatorDelivery = saved[0], saved[1], saved[2], saved[3]
				registerCoordinatorRelay, registerCoordinatorReply, registerCoordinatorAutomatic, registerCoordinatorImprovements = saved[4], saved[5], saved[6], saved[7]
			})
			runCoordinatorBackgroundPass = func(context.Context, time.Time, []func(context.Context, time.Time)) {}

			registerCoordinatorRoutes(routeParams{ctx: context.Background(), router: gin.New(), services: &Services{Coordinator: svc}, log: newTestLogger()})

			runCoordinatorBackgroundPass = origPass
			registerCoordinatorContainment, registerCoordinatorSpend, registerCoordinatorWake, registerCoordinatorDelivery = saved[0], saved[1], saved[2], saved[3]
			registerCoordinatorRelay, registerCoordinatorReply, registerCoordinatorAutomatic, registerCoordinatorImprovements = saved[4], saved[5], saved[6], saved[7]

			want := 0
			if p2 && p3 {
				want = 1
			}
			for _, n := range names {
				if called[n] != want {
					t.Fatalf("p2=%v p3=%v: %s called %d times, want %d", p2, p3, n, called[n], want)
				}
			}
		}
	}
}

func TestRegisterCoordinatorWakeState_PrunesAndSurvivesFailure(t *testing.T) {
	pool := newCoordinatorTestPool(t)
	svc, err := initCoordinatorWiring(context.Background(), pool, newCoordinatorTestTracker(t), nil, nil, nil, true, true, true, newTestLogger())
	if err != nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}
	store, err := coordinator.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	c := &coordinator.Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	old := time.Now().UTC().Add(-200 * 24 * time.Hour)
	if _, err := pool.Writer().Exec(`INSERT INTO coordinator_unattended_turns (id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, outcome, started_at, finished_at)
		VALUES ('t-old', ?, 'conv', 'sess', 1, 100, 'completed', ?, ?)`, c.ID, old, old); err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	hook := registerCoordinatorWakeState(nil, nil, svc, newTestLogger())
	hook(context.Background(), time.Now().UTC())
	var n int
	if err := pool.Reader().Get(&n, `SELECT COUNT(*) FROM coordinator_unattended_turns`); err != nil || n != 0 {
		t.Fatalf("turns after prune = %d err=%v, want 0", n, err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	core, logs := observer.New(zapcore.WarnLevel)
	observed, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	registerCoordinatorWakeState(nil, nil, svc, observed)(cancelled, time.Now().UTC())
	if logs.FilterMessage("coordinator wake state pruning failed").Len() != 1 {
		t.Fatalf("warn entries = %v, want one prune failure warning", logs.All())
	}
}

func TestRegisterCoordinatorRelayRoutes_MountsReadRouteOnlyWhenPhase3Effective(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, p3 := range []bool{false, true} {
		svc, err := initCoordinatorWiring(context.Background(), newCoordinatorTestPool(t), newCoordinatorTestTracker(t), nil, nil, nil, true, true, p3, newTestLogger())
		if err != nil {
			t.Fatalf("initCoordinatorWiring: %v", err)
		}
		router := gin.New()
		if p3 {
			registerCoordinatorRelayRoutes(router, nil, svc, newTestLogger())
		}
		mounted := false
		for _, route := range router.Routes() {
			if route.Method == "GET" && route.Path == "/api/v1/workspaces/:id/coordinators/:cid/relay/:taskId" {
				mounted = true
			}
		}
		if mounted != p3 {
			t.Fatalf("phase3=%v: relay route mounted = %v", p3, mounted)
		}
	}
}

func TestRegisterCoordinatorWakeState_FailedPruneStillStartsBackstopAndRecorder(t *testing.T) {
	pool := newCoordinatorTestPool(t)
	svc, err := initCoordinatorWiring(context.Background(), pool, newCoordinatorTestTracker(t), nil, nil, nil, true, true, true, newTestLogger())
	if err != nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}
	t.Cleanup(func() {
		svc.StopWakeRecorder()
		svc.StopWakeBackstop()
	})
	memBus := bus.NewMemoryEventBus(newTestLogger())
	t.Cleanup(memBus.Close)
	saved := coordinatorWakeSources
	coordinatorWakeSources = &coordinatorWakeReader{tasks: nil}
	t.Cleanup(func() { coordinatorWakeSources = saved })

	hook := registerCoordinatorWakeState(nil, memBus, svc, newTestLogger())
	if svc.WakeBackstopRunning() {
		t.Fatal("the backstop must wait for the startup pass")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	hook(cancelled, time.Now().UTC())
	if !svc.WakeBackstopRunning() {
		t.Fatal("a failing prune must not stop the backstop from starting")
	}
}

func TestWireCoordinatorSpend_MissingDependenciesFailClosed(t *testing.T) {
	svc, err := initCoordinatorWiring(context.Background(), newCoordinatorTestPool(t), newCoordinatorTestTracker(t), nil, nil, nil, true, true, true, newTestLogger())
	if err != nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}
	wireCoordinatorSpend(nil, svc, nil, nil)
	reading, err := svc.Spend(context.Background(), &coordinator.Coordinator{ID: "c-1", WorkspaceID: "ws-1"}, time.Now().UTC())
	if err == nil && reading.Measurable {
		t.Fatalf("spend with no ledger read as measurable: %+v", reading)
	}
}
