package dashboard_test

import (
	"errors"
	"net/http"
	"reflect"
	"testing"
)

func TestRouting_PutSeatExclusionReleasesParkedRunsWhenASwitchTurnsOff(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"switch turned off", `{"changes":[{"step_id":"s","role":"reviewer","enabled":false}]}`, []string{"ws-1"}},
		{"switch turned on", `{"changes":[{"step_id":"s","role":"reviewer","enabled":true}]}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps, fake := newRoutingTestDeps(t)
			deps.svc.SetSeatAuthorExclusionWriter(&fakeSeatWriter{})
			rec := putSeatExclusion(t, deps, tc.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
			}
			if !reflect.DeepEqual(fake.releasedParked, tc.want) {
				t.Fatalf("released = %v, want %v", fake.releasedParked, tc.want)
			}
		})
	}
}

func TestRouting_PutSeatExclusionKeepsParkedRunsWhenTheWriteFails(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	deps.svc.SetSeatAuthorExclusionWriter(&fakeSeatWriter{err: errors.New("db down")})
	putSeatExclusion(t, deps, `{"changes":[{"step_id":"s","role":"reviewer","enabled":false}]}`)
	if len(fake.releasedParked) != 0 {
		t.Fatalf("released after a failed write: %v", fake.releasedParked)
	}
}

func TestRouting_PutSeatExclusionSucceedsWhenReleasingParkedRunsFails(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.releaseErr = errors.New("db down")
	deps.svc.SetSeatAuthorExclusionWriter(&fakeSeatWriter{})
	rec := putSeatExclusion(t, deps, `{"changes":[{"step_id":"s","role":"reviewer","enabled":false}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("the switch is saved, so the request succeeds: status = %d", rec.Code)
	}
}
