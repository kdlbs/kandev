package coordinator

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	svcpkg "github.com/kandev/kandev/internal/task/service"
)

type fakeMeasures struct {
	days int
	out  *OutcomeMeasures
}

func (f *fakeMeasures) Measures(_ context.Context, _ string, days int, _ time.Time) (*OutcomeMeasures, error) {
	f.days = days
	return f.out, nil
}

func measuresHandlers(t *testing.T, phase31 bool) (*Handlers, *phase3Env, *fakeMeasures) {
	t.Helper()
	h, env := phase3Handlers(t, true)
	env.svc.phase31 = phase31
	reader := &fakeMeasures{out: &OutcomeMeasures{Days: 30}}
	env.svc.SetOutcomeMeasures(reader)
	return h, env, reader
}

func TestHTTPMeasuresWindowValidation(t *testing.T) {
	h, env, reader := measuresHandlers(t, true)
	cases := map[string]struct {
		query string
		code  int
		days  int
	}{
		"default":    {"", http.StatusOK, 30},
		"one":        {"?days=1", http.StatusOK, 1},
		"ninety":     {"?days=90", http.StatusOK, 90},
		"zero":       {"?days=0", http.StatusBadRequest, 0},
		"ninetyone":  {"?days=91", http.StatusBadRequest, 0},
		"negative":   {"?days=-3", http.StatusBadRequest, 0},
		"empty":      {"?days=", http.StatusBadRequest, 0},
		"repeated":   {"?days=3&days=4", http.StatusBadRequest, 0},
		"not an int": {"?days=3.5", http.StatusBadRequest, 0},
		"text":       {"?days=abc", http.StatusBadRequest, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reader.days = 0
			rec := runHandler(h.httpGetMeasures, http.MethodGet, "/x"+tc.query, "", workspaceParams(env.c.ID))
			if rec.Code != tc.code || reader.days != tc.days {
				t.Fatalf("status %d days %d, want %d / %d body=%s", rec.Code, reader.days, tc.code, tc.days, rec.Body.String())
			}
			if tc.code == http.StatusBadRequest && !strings.Contains(rec.Body.String(), "days") {
				t.Fatalf("400 does not name days: %s", rec.Body.String())
			}
		})
	}
}

func TestHTTPMeasuresFlagOffUnwiredAndForeignCoordinatorAre404(t *testing.T) {
	h, env, _ := measuresHandlers(t, false)
	if rec := runHandler(h.httpGetMeasures, http.MethodGet, "/x", "", workspaceParams(env.c.ID)); rec.Code != http.StatusNotFound {
		t.Fatalf("flag off = %d", rec.Code)
	}
	env.svc.phase31 = true
	if rec := runHandler(h.httpGetMeasures, http.MethodGet, "/x", "", workspaceParams("no-such")); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown coordinator = %d", rec.Code)
	}
	env.svc.SetOutcomeMeasures(nil)
	if rec := runHandler(h.httpGetMeasures, http.MethodGet, "/x", "", workspaceParams(env.c.ID)); rec.Code != http.StatusNotFound {
		t.Fatalf("unwired = %d", rec.Code)
	}
}

func TestHTTPMeasuresNonMemberIs403(t *testing.T) {
	h, env, _ := measuresHandlers(t, true)
	env.svc.authz = &fakeWorkspaceAuthorizer{err: svcpkg.ErrForbidden}
	if rec := runHandler(h.httpGetMeasures, http.MethodGet, "/x", "", workspaceParams(env.c.ID)); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}
