package subproc

import (
	"context"
	"errors"
	"expvar"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGitAdmissionPublishesClassMetrics(t *testing.T) {
	for _, name := range []string{
		"subproc_class_inflight",
		"subproc_class_waiters",
		"subproc_class_acquire_total",
		"subproc_class_acquire_wait_millis_total",
	} {
		if expvar.Get(name) == nil {
			t.Fatalf("expvar %q is not published", name)
		}
	}
}

func TestClassAdmissionRoundRobin(t *testing.T) {
	pool := NewNamedClassThrottle("class-round-robin-"+t.Name(), 1)
	hold, err := pool.AcquireClass(context.Background(), GitInteractive)
	if err != nil {
		t.Fatalf("hold slot: %v", err)
	}

	type waiter struct {
		class     GitWorkClass
		acquired  chan struct{}
		continueC chan struct{}
		err       chan error
	}
	waiters := []waiter{
		{class: GitInteractive, acquired: make(chan struct{}), continueC: make(chan struct{}), err: make(chan error, 1)},
		{class: GitLifecycle, acquired: make(chan struct{}), continueC: make(chan struct{}), err: make(chan error, 1)},
		{class: GitBackground, acquired: make(chan struct{}), continueC: make(chan struct{}), err: make(chan error, 1)},
	}
	for i := range waiters {
		w := &waiters[i]
		go func() {
			release, acquireErr := pool.AcquireClass(context.Background(), w.class)
			w.err <- acquireErr
			if acquireErr != nil {
				return
			}
			close(w.acquired)
			<-w.continueC
			release()
		}()
		waitForClassWaiters(t, pool, w.class, 1)
	}

	hold()
	for i, want := range []GitWorkClass{GitInteractive, GitLifecycle, GitBackground} {
		select {
		case <-waiters[i].acquired:
		case <-time.After(time.Second):
			t.Fatalf("waiter %s was not admitted", want)
		}
		if err := <-waiters[i].err; err != nil {
			t.Fatalf("waiter %s acquire: %v", want, err)
		}
		close(waiters[i].continueC)
	}
}

func TestClassAdmissionFIFOWithinClass(t *testing.T) {
	pool := NewNamedClassThrottle("class-fifo-"+t.Name(), 1)
	hold, err := pool.AcquireClass(context.Background(), GitInteractive)
	if err != nil {
		t.Fatalf("hold slot: %v", err)
	}
	firstAcquired := make(chan struct{})
	secondAcquired := make(chan struct{})
	firstContinue := make(chan struct{})
	secondContinue := make(chan struct{})
	startWaiter := func(acquired, continueC chan struct{}) {
		go func() {
			release, acquireErr := pool.AcquireClass(context.Background(), GitInteractive)
			if acquireErr != nil {
				t.Errorf("acquire: %v", acquireErr)
				return
			}
			close(acquired)
			<-continueC
			release()
		}()
	}
	startWaiter(firstAcquired, firstContinue)
	waitForClassWaiters(t, pool, GitInteractive, 1)
	startWaiter(secondAcquired, secondContinue)
	waitForClassWaiters(t, pool, GitInteractive, 2)

	hold()
	select {
	case <-firstAcquired:
	case <-time.After(time.Second):
		t.Fatal("first FIFO waiter was not admitted")
	}
	select {
	case <-secondAcquired:
		t.Fatal("second FIFO waiter was admitted before first release")
	default:
	}
	close(firstContinue)
	select {
	case <-secondAcquired:
	case <-time.After(time.Second):
		t.Fatal("second FIFO waiter was not admitted")
	}
	close(secondContinue)
}

func TestClassAdmissionCancellationDoesNotAdvanceQueue(t *testing.T) {
	pool := NewNamedClassThrottle("class-cancel-"+t.Name(), 1)
	hold, err := pool.AcquireClass(context.Background(), GitInteractive)
	if err != nil {
		t.Fatalf("hold slot: %v", err)
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	canceled := make(chan error, 1)
	go func() {
		_, acquireErr := pool.AcquireClass(cancelCtx, GitInteractive)
		canceled <- acquireErr
	}()
	waitForClassWaiters(t, pool, GitInteractive, 1)
	cancel()
	if err := <-canceled; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled acquire = %v, want context.Canceled", err)
	}
	waitForClassWaiters(t, pool, GitInteractive, 0)

	acquired := make(chan struct{})
	continueC := make(chan struct{})
	go func() {
		release, acquireErr := pool.AcquireClass(context.Background(), GitLifecycle)
		if acquireErr != nil {
			t.Errorf("lifecycle acquire: %v", acquireErr)
			return
		}
		close(acquired)
		<-continueC
		release()
	}()
	waitForClassWaiters(t, pool, GitLifecycle, 1)
	hold()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("lifecycle waiter was not admitted after cancellation")
	}
	close(continueC)
}

func TestClassAdmissionCancellationWinsReleaseBoundary(t *testing.T) {
	pool := NewNamedClassThrottle("class-cancel-release-boundary-"+t.Name(), 1)
	hold, err := pool.AcquireClass(context.Background(), GitInteractive)
	if err != nil {
		t.Fatalf("hold slot: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, acquireErr := pool.AcquireClass(ctx, GitInteractive)
		result <- acquireErr
	}()
	waitForClassWaiters(t, pool, GitInteractive, 1)

	// Cancel before releasing without waiting for the waiter goroutine. The
	// dispatcher must inspect the canceled queue head itself; otherwise a
	// release can grant the slot and the canceled waiter can report success.
	cancel()
	hold()

	select {
	case acquireErr := <-result:
		if !errors.Is(acquireErr, context.Canceled) {
			t.Fatalf("acquire error = %v, want context.Canceled", acquireErr)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter did not return at the release boundary")
	}
	if snapshot := pool.admission.snapshot(); snapshot.Inflight != 0 || snapshot.Waiters != 0 {
		t.Fatalf("admission after canceled release = %+v, want no slot or waiter", snapshot)
	}
}

func TestClassAdmissionCancellationCountsAggregateAndClassAttempt(t *testing.T) {
	pool := NewNamedClassThrottle("class-cancel-metrics-"+t.Name(), 1)
	hold, err := pool.AcquireClass(context.Background(), GitInteractive)
	if err != nil {
		t.Fatalf("hold slot: %v", err)
	}
	defer hold()

	beforeAggregate := metricInt(subprocAcquireTotal, pool.name)
	beforeClass := metricInt(subprocClassAcquireTotal, classMetricKey(pool.name, GitLifecycle))
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, acquireErr := pool.AcquireClass(ctx, GitLifecycle)
		result <- acquireErr
	}()
	waitForClassWaiters(t, pool, GitLifecycle, 1)
	cancel()
	if acquireErr := <-result; !errors.Is(acquireErr, context.Canceled) {
		t.Fatalf("canceled acquire = %v, want context.Canceled", acquireErr)
	}

	if got := metricInt(subprocAcquireTotal, pool.name); got != beforeAggregate+1 {
		t.Fatalf("aggregate acquire total = %d, want %d", got, beforeAggregate+1)
	}
	if got := metricInt(subprocClassAcquireTotal, classMetricKey(pool.name, GitLifecycle)); got != beforeClass+1 {
		t.Fatalf("lifecycle acquire total = %d, want %d", got, beforeClass+1)
	}
}

func TestClassAdmissionGlobalCapAndWorkConservation(t *testing.T) {
	pool := NewNamedClassThrottle("class-cap-"+t.Name(), 2)
	var active, peak int64
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		class := gitWorkClassOrder[i%len(gitWorkClassOrder)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := pool.AcquireClass(context.Background(), class)
			if err != nil {
				t.Errorf("acquire %s: %v", class, err)
				return
			}
			defer release()
			current := atomic.AddInt64(&active, 1)
			for {
				previous := atomic.LoadInt64(&peak)
				if current <= previous || atomic.CompareAndSwapInt64(&peak, previous, current) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt64(&active, -1)
		}()
	}
	wg.Wait()
	if peak > 2 {
		t.Fatalf("peak active = %d, want <= 2", peak)
	}
	if got := pool.admission.snapshot().Inflight; got != 0 {
		t.Fatalf("inflight after drain = %d, want 0", got)
	}

	holdA, err := pool.AcquireClass(context.Background(), GitInteractive)
	if err != nil {
		t.Fatalf("interactive hold A: %v", err)
	}
	holdB, err := pool.AcquireClass(context.Background(), GitInteractive)
	if err != nil {
		t.Fatalf("interactive hold B: %v", err)
	}
	queued := make(chan struct{})
	continueC := make(chan struct{})
	go func() {
		release, acquireErr := pool.AcquireClass(context.Background(), GitInteractive)
		if acquireErr != nil {
			t.Errorf("queued acquire: %v", acquireErr)
			return
		}
		close(queued)
		<-continueC
		release()
	}()
	waitForClassWaiters(t, pool, GitInteractive, 1)
	holdA()
	select {
	case <-queued:
	case <-time.After(time.Second):
		t.Fatal("queued interactive work did not use released capacity")
	}
	holdB()
	close(continueC)
}

func TestClassAdmissionSnapshot(t *testing.T) {
	pool := NewNamedClassThrottle("class-snapshot-"+t.Name(), 1)
	hold, err := pool.AcquireClass(context.Background(), GitInteractive)
	if err != nil {
		t.Fatalf("hold slot: %v", err)
	}
	queued := make(chan struct{})
	go func() {
		release, acquireErr := pool.AcquireClass(context.Background(), GitLifecycle)
		if acquireErr != nil {
			t.Errorf("queued acquire: %v", acquireErr)
			return
		}
		close(queued)
		release()
	}()
	waitForClassWaiters(t, pool, GitLifecycle, 1)
	snapshot := pool.admission.snapshot()
	if snapshot.Capacity != 1 || snapshot.Inflight != 1 || snapshot.Waiters != 1 {
		t.Fatalf("snapshot = %+v, want cap=1 inflight=1 waiters=1", snapshot)
	}
	if got := snapshot.Classes[string(GitInteractive)].Inflight; got != 1 {
		t.Fatalf("interactive snapshot inflight = %d, want 1", got)
	}
	if got := snapshot.Classes[string(GitLifecycle)].Waiters; got != 1 {
		t.Fatalf("lifecycle snapshot waiters = %d, want 1", got)
	}
	hold()
	select {
	case <-queued:
	case <-time.After(time.Second):
		t.Fatal("queued lifecycle work was not admitted")
	}
}

func TestClassAdmissionRejectsUnknownClass(t *testing.T) {
	pool := NewNamedClassThrottle("class-invalid-"+t.Name(), 1)
	_, err := pool.AcquireClass(context.Background(), GitWorkClass("unknown"))
	if !errors.Is(err, ErrInvalidGitWorkClass) {
		t.Fatalf("err = %v, want ErrInvalidGitWorkClass", err)
	}
}

func TestClassThrottleRejectsClasslessAcquire(t *testing.T) {
	pool := NewNamedClassThrottle("class-required-"+t.Name(), 1)
	if _, err := pool.Acquire(context.Background()); !errors.Is(err, ErrGitWorkClassRequired) {
		t.Fatalf("classless acquire error = %v, want ErrGitWorkClassRequired", err)
	}
}

func TestGitCapacityTestSeam(t *testing.T) {
	restore := Git().SetCapForTest(3)
	defer restore()
	if got := GitCapacity(); got != 3 {
		t.Fatalf("GitCapacity() = %d, want 3", got)
	}
}

func TestRunGitOutputAfterAcquireStartsTimeoutAfterAdmission(t *testing.T) {
	restore := Git().SetCapForTest(1)
	t.Cleanup(restore)
	hold, err := AcquireGit(context.Background(), GitInteractive)
	if err != nil {
		t.Fatalf("hold slot: %v", err)
	}

	type result struct {
		out     []byte
		runErr  error
		execErr error
	}
	resultCh := make(chan result, 1)
	started := make(chan struct{})
	go func() {
		out, runErr, execErr := RunGitOutputAfterAcquire(context.Background(), GitInteractive, 50*time.Millisecond, func(execCtx context.Context) *exec.Cmd {
			close(started)
			return exec.CommandContext(execCtx, "sh", "-c", "printf ok")
		})
		resultCh <- result{out: out, runErr: runErr, execErr: execErr}
	}()
	waitForClassWaiters(t, Git(), GitInteractive, 1)
	hold()

	select {
	case got := <-resultCh:
		if got.runErr != nil || got.execErr != nil || strings.TrimSpace(string(got.out)) != "ok" {
			t.Fatalf("result = %+v, want successful command after queued admission", got)
		}
	case <-time.After(time.Second):
		t.Fatal("queued command did not complete after admission")
	}
	select {
	case <-started:
	default:
		t.Fatal("command builder did not run after admission")
	}
}

func TestRunGitOutputAfterAcquireWithExecutionContextDetachesAdmissionDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX `true` binary as a no-op subprocess")
	}
	restore := Git().SetCapForTest(1)
	t.Cleanup(restore)
	hold, err := AcquireGit(context.Background(), GitInteractive)
	if err != nil {
		t.Fatalf("hold slot: %v", err)
	}

	acquireCtx, cancelAcquire := context.WithTimeout(context.Background(), 350*time.Millisecond)
	t.Cleanup(cancelAcquire)
	var remaining time.Duration
	resultCh := make(chan error, 1)
	go func() {
		_, runErr, execErr := RunGitOutputAfterAcquireWithExecutionContext(
			acquireCtx,
			context.Background(),
			GitInteractive,
			100*time.Millisecond,
			func(execCtx context.Context) *exec.Cmd {
				if deadline, ok := execCtx.Deadline(); ok {
					remaining = time.Until(deadline)
				}
				return exec.CommandContext(execCtx, "true")
			},
		)
		if runErr != nil {
			resultCh <- runErr
			return
		}
		resultCh <- execErr
	}()
	waitForClassWaiters(t, Git(), GitInteractive, 1)
	timer := time.NewTimer(300 * time.Millisecond)
	<-timer.C
	hold()

	if err := <-resultCh; err != nil {
		t.Fatalf("queued command failed: %v", err)
	}
	if remaining < 80*time.Millisecond {
		t.Fatalf("execution deadline has only %v remaining; admission deadline leaked into execution", remaining)
	}
}

func TestRunGitOutputAfterAcquireMarksAdmissionCancellation(t *testing.T) {
	restore := Git().SetCapForTest(1)
	t.Cleanup(restore)
	hold, err := AcquireGit(context.Background(), GitInteractive)
	if err != nil {
		t.Fatalf("hold slot: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	go func() {
		_, runErr, execErr := RunGitOutputAfterAcquire(ctx, GitInteractive, time.Second, func(execCtx context.Context) *exec.Cmd {
			return exec.CommandContext(execCtx, "sh", "-c", "printf unexpected")
		})
		if execErr != nil {
			resultCh <- execErr
			return
		}
		resultCh <- runErr
	}()
	waitForClassWaiters(t, Git(), GitInteractive, 1)
	cancel()
	select {
	case err := <-resultCh:
		if !errors.Is(err, ErrAdmissionCanceled) || !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want admission cancellation wrapping context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled admission did not return")
	}
	hold()
}

func waitForClassWaiters(t *testing.T, pool *Throttle, class GitWorkClass, want int64) {
	t.Helper()
	key := classMetricKey(pool.name, class)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := metricInt(subprocClassWaiters, key); got == want {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("waiters for %s did not reach %d (got %d)", class, want, metricInt(subprocClassWaiters, key))
}
