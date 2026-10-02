package coordinator

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// fakeSpendLedger records every SumUsageForTasks call and answers from
// per-call hooks so a test can fail one chunk or one window.
type fakeSpendLedger struct {
	mu        sync.Mutex
	taskCalls []spendCall
	sumFn     func(call spendCall) (taskmodels.UsageSum, error)
	turnCalls []turnCall
	turnFn    func(call turnCall) (int64, error)
}

type spendCall struct {
	IDs      []string
	From, To time.Time
}

type turnCall struct {
	SessionID, TurnID string
	NotAfter          time.Time
}

func (f *fakeSpendLedger) SumUsageForTasks(_ context.Context, ids []string, from, to time.Time) (taskmodels.UsageSum, error) {
	f.mu.Lock()
	call := spendCall{IDs: append([]string(nil), ids...), From: from, To: to}
	f.taskCalls = append(f.taskCalls, call)
	f.mu.Unlock()
	if f.sumFn == nil {
		return taskmodels.UsageSum{}, nil
	}
	return f.sumFn(call)
}

func (f *fakeSpendLedger) SumUsageForTurn(_ context.Context, sessionID, turnID string, notAfter time.Time) (int64, error) {
	f.mu.Lock()
	call := turnCall{sessionID, turnID, notAfter}
	f.turnCalls = append(f.turnCalls, call)
	f.mu.Unlock()
	if f.turnFn == nil {
		return 0, nil
	}
	return f.turnFn(call)
}

var spendNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func spendCoordinator() *Coordinator {
	return &Coordinator{ID: "coord-1", WorkspaceID: testWorkspaceID}
}

func spendConvTask(id, coordinatorID string, archived bool) *taskmodels.Task {
	task := &taskmodels.Task{ID: id, WorkspaceID: testWorkspaceID, CreatedAt: spendNow}
	if coordinatorID != "" {
		task.Metadata = map[string]interface{}{taskmodels.MetaKeyCoordinatorID: coordinatorID}
	}
	if archived {
		at := spendNow
		task.ArchivedAt = &at
	}
	return task
}

func newSpendService(t *testing.T, tasks ...*taskmodels.Task) (*Service, *fakeConversationTasks, *fakeSpendLedger) {
	t.Helper()
	svc := newServiceForTest(t, nil, nil, nil)
	conv := newFakeConversationTasks()
	for _, task := range tasks {
		conv.tasks[task.ID] = task
	}
	ledger := &fakeSpendLedger{}
	svc.conversationTasks = conv
	svc.SetSpendDeps(ledger, nil, nil)
	return svc, conv, ledger
}

func readCounter(name, key string) int64 {
	return spendCounterValue(name, key)
}

func TestSpend_SumsOnlyThisCoordinatorsConversationTasksCurrentAndArchived(t *testing.T) {
	svc, _, ledger := newSpendService(t,
		spendConvTask("cur", "coord-1", false),
		spendConvTask("old", "coord-1", true),
		spendConvTask("other", "coord-2", false),
		spendConvTask("plain", "", false),
	)
	ledger.sumFn = func(c spendCall) (taskmodels.UsageSum, error) {
		return taskmodels.UsageSum{CostSubcents: int64(len(c.IDs)) * 70}, nil
	}

	got, err := svc.Spend(t.Context(), spendCoordinator(), spendNow)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Measurable || got.Degraded || got.WindowSubcents != 140 {
		t.Fatalf("reading = %+v", got)
	}
	for _, c := range ledger.taskCalls {
		if len(c.IDs) != 2 {
			t.Fatalf("call ids = %v, want exactly cur and old", c.IDs)
		}
	}
}

func TestSpend_WindowsAreHalfOpenAnd24hAnd7d(t *testing.T) {
	svc, _, ledger := newSpendService(t, spendConvTask("cur", "coord-1", false))
	ledger.sumFn = func(c spendCall) (taskmodels.UsageSum, error) {
		if c.To.Sub(c.From) == 7*24*time.Hour {
			return taskmodels.UsageSum{CostSubcents: 71}, nil
		}
		return taskmodels.UsageSum{CostSubcents: 10}, nil
	}
	got, err := svc.Spend(t.Context(), spendCoordinator(), spendNow)
	if err != nil {
		t.Fatal(err)
	}
	if got.WindowSubcents != 10 || got.Mean7dSubcents != 10 || !got.Mean7dKnown {
		t.Fatalf("reading = %+v (mean is integer 71/7)", got)
	}
	if len(ledger.taskCalls) != 2 {
		t.Fatalf("calls = %d, want window then mean", len(ledger.taskCalls))
	}
	win, mean := ledger.taskCalls[0], ledger.taskCalls[1]
	if !win.From.Equal(spendNow.Add(-24*time.Hour)) || !win.To.Equal(spendNow) {
		t.Fatalf("window = [%v,%v)", win.From, win.To)
	}
	if !mean.From.Equal(spendNow.Add(-7*24*time.Hour)) || !mean.To.Equal(spendNow) {
		t.Fatalf("mean window = [%v,%v)", mean.From, mean.To)
	}
}

func TestSpend_NoConversationTasksIsMeasurableZeroWithoutQuery(t *testing.T) {
	svc, _, ledger := newSpendService(t, spendConvTask("other", "coord-2", false))
	got, err := svc.Spend(t.Context(), spendCoordinator(), spendNow)
	if err != nil || !got.Measurable || got.WindowSubcents != 0 || !got.Mean7dKnown {
		t.Fatalf("got %+v err %v", got, err)
	}
	if len(ledger.taskCalls) != 0 {
		t.Fatalf("queried the ledger %d times for an empty task set", len(ledger.taskCalls))
	}
}

func TestSpend_ChunksAtMost500IDsAndAddsSums(t *testing.T) {
	var tasks []*taskmodels.Task
	for i := 0; i < 1001; i++ {
		tasks = append(tasks, spendConvTask(fmt.Sprintf("t%04d", i), "coord-1", i%2 == 0))
	}
	svc, _, ledger := newSpendService(t, tasks...)
	ledger.sumFn = func(c spendCall) (taskmodels.UsageSum, error) {
		if len(c.IDs) > 500 {
			return taskmodels.UsageSum{}, errors.New("too many ids")
		}
		return taskmodels.UsageSum{CostSubcents: int64(len(c.IDs))}, nil
	}
	got, err := svc.Spend(t.Context(), spendCoordinator(), spendNow)
	if err != nil || got.WindowSubcents != 1001 {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestSpend_SumsSaturateAtMaxInt64(t *testing.T) {
	var tasks []*taskmodels.Task
	for i := 0; i < 600; i++ {
		tasks = append(tasks, spendConvTask(fmt.Sprintf("t%03d", i), "coord-1", false))
	}
	svc, _, ledger := newSpendService(t, tasks...)
	ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) {
		return taskmodels.UsageSum{CostSubcents: math.MaxInt64 - 5}, nil
	}
	got, err := svc.Spend(t.Context(), spendCoordinator(), spendNow)
	if err != nil || got.WindowSubcents != math.MaxInt64 || !got.Measurable {
		t.Fatalf("got %+v err %v", got, err)
	}
	if CheckCeilingNotReached(got, 1) {
		t.Fatal("a saturated window must read as at or above any ceiling")
	}
}

func TestSpend_UnpricedRowIsDegradedNotAnErrorAndKeepsMean(t *testing.T) {
	svc, _, ledger := newSpendService(t, spendConvTask("cur", "coord-1", false))
	ledger.sumFn = func(c spendCall) (taskmodels.UsageSum, error) {
		if c.To.Sub(c.From) == 7*24*time.Hour {
			return taskmodels.UsageSum{CostSubcents: 700, HasUnpriced: true}, nil
		}
		return taskmodels.UsageSum{CostSubcents: 33, HasUnpriced: true}, nil
	}
	got, err := svc.Spend(t.Context(), spendCoordinator(), spendNow)
	if err != nil {
		t.Fatalf("degraded is not an error: %v", err)
	}
	if got.Measurable || !got.Degraded || got.WindowSubcents != 33 || got.Mean7dSubcents != 100 || !got.Mean7dKnown {
		t.Fatalf("reading = %+v", got)
	}
}

func TestSpend_FailedTaskListIsUnmeasurableWithZeroAmounts(t *testing.T) {
	svc, conv, ledger := newSpendService(t, spendConvTask("cur", "coord-1", false))
	conv.listErr = errors.New("boom")
	before := readCounter("coordinator_spend_read_failed_total", "tasks")
	got, err := svc.Spend(t.Context(), spendCoordinator(), spendNow)
	if err == nil || got != (SpendReading{}) {
		t.Fatalf("got %+v err %v", got, err)
	}
	if len(ledger.taskCalls) != 0 {
		t.Fatal("ledger queried after the task list failed")
	}
	if d := readCounter("coordinator_spend_read_failed_total", "tasks") - before; d != 1 {
		t.Fatalf("tasks failure counter delta = %d", d)
	}
}

func TestSpend_FailedChunkDiscardsPartialTotalAndSkipsMean(t *testing.T) {
	var tasks []*taskmodels.Task
	for i := 0; i < 501; i++ {
		tasks = append(tasks, spendConvTask(fmt.Sprintf("t%03d", i), "coord-1", false))
	}
	svc, _, ledger := newSpendService(t, tasks...)
	n := 0
	ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) {
		n++
		if n == 2 {
			return taskmodels.UsageSum{}, errors.New("chunk failed")
		}
		return taskmodels.UsageSum{CostSubcents: 9}, nil
	}
	before := readCounter("coordinator_spend_read_failed_total", "window")
	got, err := svc.Spend(t.Context(), spendCoordinator(), spendNow)
	if err == nil || got != (SpendReading{}) {
		t.Fatalf("got %+v err %v", got, err)
	}
	if n != 2 {
		t.Fatalf("calls after failure = %d, the 7-day read must not run", n)
	}
	if d := readCounter("coordinator_spend_read_failed_total", "window") - before; d != 1 {
		t.Fatalf("window failure counter delta = %d", d)
	}
}

func TestSpend_FailedMeanLeavesSpendMeasurable(t *testing.T) {
	svc, _, ledger := newSpendService(t, spendConvTask("cur", "coord-1", false))
	ledger.sumFn = func(c spendCall) (taskmodels.UsageSum, error) {
		if c.To.Sub(c.From) == 7*24*time.Hour {
			return taskmodels.UsageSum{}, errors.New("mean failed")
		}
		return taskmodels.UsageSum{CostSubcents: 12}, nil
	}
	before := readCounter("coordinator_spend_read_failed_total", "mean")
	got, err := svc.Spend(t.Context(), spendCoordinator(), spendNow)
	if err != nil || !got.Measurable || got.WindowSubcents != 12 || got.Mean7dKnown || got.Mean7dSubcents != 0 {
		t.Fatalf("got %+v err %v", got, err)
	}
	if d := readCounter("coordinator_spend_read_failed_total", "mean") - before; d != 1 {
		t.Fatalf("mean failure counter delta = %d", d)
	}
}

func TestSpend_ScopeErrorsRunNoQueryAndAreNotCountedAsFailedReads(t *testing.T) {
	for name, c := range map[string]*Coordinator{
		"empty coordinator": {ID: "", WorkspaceID: testWorkspaceID},
		"empty workspace":   {ID: "coord-1", WorkspaceID: ""},
		"nil coordinator":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			svc, conv, ledger := newSpendService(t, spendConvTask("cur", "coord-1", false))
			conv.listErr = errors.New("must not be called")
			var before [3]int64
			for i, k := range []string{"tasks", "window", "mean"} {
				before[i] = readCounter("coordinator_spend_read_failed_total", k)
			}
			got, err := svc.Spend(t.Context(), c, spendNow)
			if !errors.Is(err, ErrSpendScope) || got != (SpendReading{}) {
				t.Fatalf("got %+v err %v", got, err)
			}
			if len(ledger.taskCalls) != 0 {
				t.Fatal("scope error ran a query")
			}
			for i, k := range []string{"tasks", "window", "mean"} {
				if readCounter("coordinator_spend_read_failed_total", k) != before[i] {
					t.Fatalf("scope error counted under %s", k)
				}
			}
		})
	}
}

func TestSpend_MissingDependenciesFailClosed(t *testing.T) {
	svc := newServiceForTest(t, nil, nil, nil)
	got, err := svc.Spend(t.Context(), spendCoordinator(), spendNow)
	if err == nil || got.Measurable {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestCheckSpendMeasurable(t *testing.T) {
	cases := []struct {
		name string
		r    SpendReading
		err  error
		want bool
	}{
		{"measurable", SpendReading{Measurable: true}, nil, true},
		{"degraded", SpendReading{Degraded: true}, nil, false},
		{"error wins over a measurable flag", SpendReading{Measurable: true}, errors.New("x"), false},
	}
	for _, tc := range cases {
		if got := CheckSpendMeasurable(tc.r, tc.err); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestCheckCeilingNotReached(t *testing.T) {
	cases := []struct {
		name    string
		r       SpendReading
		ceiling int64
		want    bool
	}{
		{"below", SpendReading{Measurable: true, WindowSubcents: 99}, 100, true},
		{"equal is reached", SpendReading{Measurable: true, WindowSubcents: 100}, 100, false},
		{"above", SpendReading{Measurable: true, WindowSubcents: 101}, 100, false},
		{"unmeasurable is never within budget", SpendReading{WindowSubcents: 1}, 100, false},
	}
	for _, tc := range cases {
		if got := CheckCeilingNotReached(tc.r, tc.ceiling); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestTurnCost(t *testing.T) {
	finished := spendNow
	key := TurnKey{SessionID: "s1", SessionTurnID: "turn-1", FinishedAt: finished}

	t.Run("sums up to ten minutes past the settle inclusive", func(t *testing.T) {
		svc, _, ledger := newSpendService(t)
		ledger.turnFn = func(turnCall) (int64, error) { return 55, nil }
		cost, known, err := svc.TurnCost(t.Context(), key)
		if err != nil || !known || cost != 55 {
			t.Fatalf("cost %d known %v err %v", cost, known, err)
		}
		c := ledger.turnCalls[0]
		if c.SessionID != "s1" || c.TurnID != "turn-1" || !c.NotAfter.Equal(finished.Add(10*time.Minute)) {
			t.Fatalf("call = %+v", c)
		}
	})
	t.Run("no rows is a known zero", func(t *testing.T) {
		svc, _, _ := newSpendService(t)
		cost, known, err := svc.TurnCost(t.Context(), key)
		if err != nil || !known || cost != 0 {
			t.Fatalf("cost %d known %v err %v", cost, known, err)
		}
	})
	t.Run("empty ids are unknown with no query", func(t *testing.T) {
		svc, _, ledger := newSpendService(t)
		for _, k := range []TurnKey{{SessionTurnID: "t", FinishedAt: finished}, {SessionID: "s", FinishedAt: finished}} {
			cost, known, err := svc.TurnCost(t.Context(), k)
			if err != nil || known || cost != 0 {
				t.Fatalf("cost %d known %v err %v", cost, known, err)
			}
		}
		if len(ledger.turnCalls) != 0 {
			t.Fatal("queried with an empty id")
		}
	})
	t.Run("a read error is unknown", func(t *testing.T) {
		svc, _, ledger := newSpendService(t)
		ledger.turnFn = func(turnCall) (int64, error) { return 9, errors.New("boom") }
		cost, known, err := svc.TurnCost(t.Context(), key)
		if err == nil || known || cost != 0 {
			t.Fatalf("cost %d known %v err %v", cost, known, err)
		}
	})
	t.Run("missing ledger is an error", func(t *testing.T) {
		svc := newServiceForTest(t, nil, nil, nil)
		if _, known, err := svc.TurnCost(t.Context(), key); err == nil || known {
			t.Fatalf("known %v err %v", known, err)
		}
	})
}
