package coordinator

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	svcpkg "github.com/kandev/kandev/internal/task/service"
)

type fakeNames struct {
	name string
	ok   bool
	err  error
}

func (f fakeNames) UserDisplayName(context.Context, string) (string, bool, error) {
	return f.name, f.ok, f.err
}

func pauseEnv(t *testing.T) *phase3Env {
	t.Helper()
	env := newPhase3Env(t, true)
	env.svc.phase31 = true
	t.Cleanup(env.svc.StopPause)
	return env
}

func (e *phase3Env) setPaused(t *testing.T, paused bool) (*Coordinator, error) {
	t.Helper()
	body := `{"paused":false}`
	if paused {
		body = `{"paused":true}`
	}
	return e.svc.SetPaused(context.Background(), phase3Workspace, e.c.ID, []byte(body))
}

func TestSetPaused_PauseResumeIdempotentAndPublishesOnce(t *testing.T) {
	env := pauseEnv(t)
	got, err := env.setPaused(t, true)
	if err != nil || got.PausedAt == nil {
		t.Fatalf("pause = %+v, %v", got, err)
	}
	first := *got.PausedAt
	again, err := env.setPaused(t, true)
	if err != nil || !again.PausedAt.Equal(first) {
		t.Fatalf("second pause = %+v, %v", again, err)
	}
	waitForEvents(t, &env.events, 1)
	if n := len(env.events.snapshot()); n != 1 || !env.events.snapshot()[0].AutonomyChanged {
		t.Fatalf("events after double pause = %+v", env.events.snapshot())
	}
	if !env.svc.knownPaused.Has(env.c.ID) {
		t.Fatal("known-paused set lacks the paused coordinator")
	}
	if got, err = env.setPaused(t, false); err != nil || got.PausedAt != nil {
		t.Fatalf("resume = %+v, %v", got, err)
	}
	if _, err = env.setPaused(t, false); err != nil {
		t.Fatal(err)
	}
	waitForEvents(t, &env.events, 2)
	if n := len(env.events.snapshot()); n != 2 {
		t.Fatalf("events = %d, want 2 (no-op resume publishes nothing)", n)
	}
	if env.svc.knownPaused.Has(env.c.ID) {
		t.Fatal("known-paused set kept a resumed coordinator")
	}
}

func TestSetPaused_RecordsManagerAndLeavesSettings(t *testing.T) {
	env := pauseEnv(t)
	mustExec(t, env.store, `UPDATE coordinators SET autonomy_enabled = 1, cost_ceiling_subcents = 500 WHERE id = ?`, env.c.ID)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "u-1"})
	got, err := env.svc.SetPaused(ctx, phase3Workspace, env.c.ID, []byte(`{"paused":true}`))
	if err != nil || got.PausedBy != "u-1" {
		t.Fatalf("pause = %+v, %v", got, err)
	}
	if !got.AutonomyEnabled || got.CostCeilingSubcents == nil || *got.CostCeilingSubcents != 500 {
		t.Fatalf("pause changed settings: %+v", got)
	}
}

func TestSetPaused_FlagOffIs404(t *testing.T) {
	env := newPhase3Env(t, true)
	_, err := env.setPaused(t, true)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if env.reload(t).PausedAt != nil {
		t.Fatal("paused with the flag off")
	}
}

func TestSetPaused_CheckOrder(t *testing.T) {
	env := pauseEnv(t)
	if _, err := env.svc.SetPaused(context.Background(), phase3Workspace, "missing", []byte(`{}`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown coordinator = %v, want 404 before the body check", err)
	}
	env.svc.authz = &fakeWorkspaceAuthorizer{err: svcpkg.ErrForbidden}
	if _, err := env.svc.SetPaused(context.Background(), phase3Workspace, env.c.ID, []byte(`{}`)); err == nil || errors.As(err, new(*FieldError)) {
		t.Fatalf("reader with a bad body = %v, want 403 before the body check", err)
	}
}

func TestSetPaused_BadBodyIs400AndChangesNothing(t *testing.T) {
	env := pauseEnv(t)
	for _, body := range []string{`{}`, `{"paused":"yes"}`, `{"paused":null}`, `nope`, ``} {
		_, err := env.svc.SetPaused(context.Background(), phase3Workspace, env.c.ID, []byte(body))
		assertFieldError(t, err, "paused")
	}
	if env.reload(t).PausedAt != nil {
		t.Fatal("a refused body paused the coordinator")
	}
}

func TestPauseView_NameResolution(t *testing.T) {
	env := pauseEnv(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "u-1"})
	c, err := env.svc.SetPaused(ctx, phase3Workspace, env.c.ID, []byte(`{"paused":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if v := env.svc.pauseView(ctx, c); !v.Paused || v.PausedAt == nil || v.PausedBy != nil {
		t.Fatalf("no resolver = %+v", v)
	}
	env.svc.SetPauserNames(fakeNames{name: "Ada", ok: true})
	if v := env.svc.pauseView(ctx, c); v.PausedBy == nil || v.PausedBy.Name != "Ada" || v.PausedBy.ID != "u-1" {
		t.Fatalf("resolved = %+v", v)
	}
	env.svc.SetPauserNames(fakeNames{ok: false})
	if v := env.svc.pauseView(ctx, c); v.PausedBy != nil {
		t.Fatalf("gone user = %+v", v)
	}
	env.svc.SetPauserNames(fakeNames{err: errors.New("db")})
	if v := env.svc.pauseView(ctx, c); !v.Paused || v.PausedBy != nil {
		t.Fatalf("unreadable = %+v", v)
	}
	if v := env.svc.pauseView(ctx, env.reload(t)); !v.Paused {
		t.Fatalf("reloaded = %+v", v)
	}
	if _, err = env.setPaused(t, false); err != nil {
		t.Fatal(err)
	}
	if v := env.svc.pauseView(ctx, env.reload(t)); v.Paused || v.PausedAt != nil || v.PausedBy != nil {
		t.Fatalf("resumed view = %+v", v)
	}
}

func TestHTTPPutPause(t *testing.T) {
	h, env := phase3Handlers(t, true)
	env.svc.phase31 = true
	t.Cleanup(env.svc.StopPause)
	params := workspaceParams(env.c.ID)
	rec := runHandler(h.httpPutPause, http.MethodPut, "/x", `{"paused":true}`, params)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"paused":true`) {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec = runHandler(h.httpPutPause, http.MethodPut, "/x", `{}`, params); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad body = %d", rec.Code)
	}
	if rec = runHandler(h.httpPutPause, http.MethodPut, "/x", `{"paused":true}`, workspaceParams("nope")); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown coordinator = %d", rec.Code)
	}
	env.svc.authz = &fakeWorkspaceAuthorizer{err: svcpkg.ErrForbidden}
	if rec = runHandler(h.httpPutPause, http.MethodPut, "/x", `{"paused":false}`, params); rec.Code != http.StatusForbidden {
		t.Fatalf("reader = %d", rec.Code)
	}
	if env.reload(t).PausedAt == nil {
		t.Fatal("a refused request changed the state")
	}
}

func TestHTTPCoordinatorReadsCarryPausedWhileFlagOff(t *testing.T) {
	h, env := phase3Handlers(t, true)
	if _, err := env.store.SetPaused(context.Background(), env.c.ID, true, "u-1"); err != nil {
		t.Fatal(err)
	}
	rec := runHandler(h.httpGetCoordinator, http.MethodGet, "/x", "", workspaceParams(env.c.ID))
	if !strings.Contains(rec.Body.String(), `"paused":true`) {
		t.Fatalf("get body = %s", rec.Body.String())
	}
}
