package instance

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/pkg/agent"
)

func newIsolationManager(t *testing.T, ports config.PortConfig) *Manager {
	t.Helper()
	previous := subproc.ManagedGitIsolation()
	subproc.SetManagedGitIsolation(false)
	t.Cleanup(func() { subproc.SetManagedGitIsolation(previous) })
	mgr := NewManager(&config.Config{
		Ports:    ports,
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.Shutdown(context.Background()) })
	return mgr
}

// admit registers a provisional bundle for id in the given mode.
func admit(t *testing.T, mgr *Manager, id string, isolation bool) *provisionalInstance {
	t.Helper()
	adm, err := mgr.admitCreate(context.Background(), &CreateRequest{ID: id, GitIsolation: isolation})
	if err != nil {
		t.Fatalf("admitCreate(%s, isolation=%v) = %v", id, isolation, err)
	}
	return adm.bundle
}

// release finishes the creation of a bundle that did not publish.
func release(t *testing.T, mgr *Manager, b *provisionalInstance) {
	t.Helper()
	if err := mgr.abandonPartialInstance(b); err != nil {
		t.Fatalf("abandonPartialInstance(%s) = %v", b.id, err)
	}
	mgr.activeCreations.Done()
}

func TestAdmitCreateAdoptsModeOnEmptyProcess(t *testing.T) {
	mgr := newIsolationManager(t, config.PortConfig{Base: 38100, Max: 38140})
	b := admit(t, mgr, "a", true)
	if !subproc.ManagedGitIsolation() {
		t.Fatal("first admitted create on an empty process must adopt its mode (on)")
	}
	release(t, mgr, b)
	if subproc.ManagedGitIsolation() {
		t.Fatal("a process whose last instance is gone must return to off")
	}
	b = admit(t, mgr, "b", false)
	if subproc.ManagedGitIsolation() {
		t.Fatal("the next admitted create must adopt its own mode (off)")
	}
	release(t, mgr, b)
}

func TestAdmitCreateRefusesOtherModeWithoutReserving(t *testing.T) {
	mgr := newIsolationManager(t, config.PortConfig{Base: 38100, Max: 38140})
	b := admit(t, mgr, "a", true)

	_, err := mgr.admitCreate(context.Background(), &CreateRequest{ID: "b", GitIsolation: false})
	if !errors.Is(err, ErrGitIsolationMismatch) {
		t.Fatalf("other-mode create = %v, want ErrGitIsolationMismatch", err)
	}
	msg := err.Error()
	for _, want := range []string{"on", "off", "stopped first", "restart"} {
		if !strings.Contains(msg, want) {
			t.Errorf("mismatch text %q does not contain %q", msg, want)
		}
	}
	mgr.mu.RLock()
	provisional := len(mgr.provisional)
	mgr.mu.RUnlock()
	if provisional != 1 {
		t.Errorf("refused create reserved a bundle: provisional=%d, want 1", provisional)
	}
	if !subproc.ManagedGitIsolation() {
		t.Error("a refused create must leave the mode as it was (on)")
	}
	release(t, mgr, b)
}

func TestAdmitCreateRefusesOtherModeAgainstPublishedInstance(t *testing.T) {
	mgr := newIsolationManager(t, config.PortConfig{Base: 38100, Max: 38140})
	subproc.SetManagedGitIsolation(true)
	mgr.instances["probe"] = &Instance{ID: "probe"}
	if _, err := mgr.admitCreate(context.Background(), &CreateRequest{ID: "x", GitIsolation: false}); !errors.Is(err, ErrGitIsolationMismatch) {
		t.Fatalf("create against a live published instance of the other mode = %v", err)
	}
}

func TestAdmitCreateConcurrentOppositeModesAdmitExactlyOne(t *testing.T) {
	mgr := newIsolationManager(t, config.PortConfig{Base: 38100, Max: 38140})
	var wg sync.WaitGroup
	results := make([]*createAdmission, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = mgr.admitCreate(context.Background(), &CreateRequest{ID: string(rune('a' + i)), GitIsolation: i == 0})
		}(i)
	}
	wg.Wait()
	admitted := 0
	for i := range results {
		if errs[i] == nil {
			admitted++
			release(t, mgr, results[i].bundle)
		} else if !errors.Is(errs[i], ErrGitIsolationMismatch) {
			t.Errorf("create %d failed with %v, want the mismatch refusal", i, errs[i])
		}
	}
	if admitted != 1 {
		t.Fatalf("admitted %d opposite-mode creates against an empty process, want exactly 1", admitted)
	}
}

func TestSameModeFailureKeepsModeWhileOverlapLive(t *testing.T) {
	mgr := newIsolationManager(t, config.PortConfig{Base: 38100, Max: 38140})
	first := admit(t, mgr, "a", true)
	second := admit(t, mgr, "b", true)

	release(t, mgr, first)
	if !subproc.ManagedGitIsolation() {
		t.Fatal("the first create failing must not flip the mode under an overlapping same-mode create")
	}
	release(t, mgr, second)
	if subproc.ManagedGitIsolation() {
		t.Fatal("the last live instance leaving must return the process to off")
	}
}

func TestCleanupFailedBundleKeepsOtherModeRefused(t *testing.T) {
	mgr := newIsolationManager(t, config.PortConfig{Base: 38100, Max: 38140})
	b := admit(t, mgr, "a", true)
	// A bundle with no listener cannot finish cleanup, so it stays provisional.
	_ = b.listener.Close()
	b.listener = nil
	if err := mgr.abandonPartialInstance(b); err == nil {
		t.Fatal("cleanup of a bundle without a listener must fail")
	}
	if _, err := mgr.admitCreate(context.Background(), &CreateRequest{ID: "b", GitIsolation: false}); !errors.Is(err, ErrGitIsolationMismatch) {
		t.Fatalf("other-mode create against a cleanup-failed bundle = %v, want refusal", err)
	}
	if !subproc.ManagedGitIsolation() {
		t.Fatal("a cleanup-failed bundle must keep the mode held")
	}
	mgr.activeCreations.Done()
}

func TestAdmitCreateDuplicateIDKeepsItsTextAndMode(t *testing.T) {
	mgr := newIsolationManager(t, config.PortConfig{Base: 38100, Max: 38140})
	b := admit(t, mgr, "prov", true)
	mgr.instances["pub"] = &Instance{ID: "pub"}

	// The duplicate-ID check runs before the mode check, so even a request of
	// the other mode gets the existing ID texts.
	_, err := mgr.admitCreate(context.Background(), &CreateRequest{ID: "pub", GitIsolation: false})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("published duplicate = %v, want the already-exists text", err)
	}
	_, err = mgr.admitCreate(context.Background(), &CreateRequest{ID: "prov", GitIsolation: false})
	if err == nil || !strings.Contains(err.Error(), "is still cleaning up") {
		t.Fatalf("provisional duplicate = %v, want the still-cleaning-up text", err)
	}
	if !subproc.ManagedGitIsolation() {
		t.Error("an ID collision must not change the mode")
	}
	delete(mgr.instances, "pub")
	release(t, mgr, b)
}

func TestAdmitCreateFailuresLeaveEmptyProcessOff(t *testing.T) {
	t.Run("port allocation failure", func(t *testing.T) {
		mgr := newIsolationManager(t, config.PortConfig{Base: 1, Max: 0})
		if _, err := mgr.admitCreate(context.Background(), &CreateRequest{ID: "a", GitIsolation: true}); err == nil {
			t.Fatal("expected a port allocation failure")
		}
		if subproc.ManagedGitIsolation() {
			t.Fatal("a failed port allocation on an empty process must leave it off")
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		mgr := newIsolationManager(t, config.PortConfig{Base: 38100, Max: 38140})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := mgr.admitCreate(ctx, &CreateRequest{ID: "a", GitIsolation: true}); err == nil {
			t.Fatal("expected a cancelled-context failure")
		}
		if subproc.ManagedGitIsolation() {
			t.Fatal("a cancelled create on an empty process must leave it off")
		}
	})
}

func TestStopInstanceClearsModeWhenLastInstanceGone(t *testing.T) {
	mgr := newIsolationManager(t, config.PortConfig{Base: 38100, Max: 38140})
	subproc.SetManagedGitIsolation(true)
	inst := &Instance{ID: "a"}
	mgr.instances["a"] = inst
	if err := mgr.stopInstance(context.Background(), "a", inst); err != nil {
		t.Fatalf("stopInstance: %v", err)
	}
	if subproc.ManagedGitIsolation() {
		t.Fatal("stopping the last instance must return the process to off")
	}
}
