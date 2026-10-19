package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/common/httpmw"
)

// fakeAccountBlockReleaser holds a set of execution profiles, the way the
// runtime circuit registry does, and records which profiles a release reached.
type fakeAccountBlockReleaser struct {
	held    map[string]bool
	reached []string
	err     error
}

func (f *fakeAccountBlockReleaser) ReleaseAccountBlock(_ context.Context, id string) (bool, error) {
	f.reached = append(f.reached, id)
	if f.err != nil {
		return false, f.err
	}
	released := f.held[id]
	delete(f.held, id)
	return released, nil
}

func releaseAccountBlock(t *testing.T, router http.Handler, id string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-profiles/"+id+"/account-block/release", nil)
	request.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	body := map[string]any{}
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	return response.Code, body
}

func TestReleaseAccountBlockEndpoint(t *testing.T) {
	repo := newFakeSettingsRepo()
	blocked := seedProfile(repo, "blocked", "agent-1", "Blocked", "")
	seedProfile(repo, "sibling", "agent-1", "Sibling", "")
	seedProfile(repo, "office", "agent-1", "Office", "workspace-1")
	router, ctrl, _ := newSettingsHarness(t, repo, nil)

	if code, _ := releaseAccountBlock(t, router, "blocked"); code != http.StatusServiceUnavailable {
		t.Fatalf("unwired release status = %d, want 503", code)
	}

	releaser := &fakeAccountBlockReleaser{held: map[string]bool{"blocked": true, "sibling": false}}
	ctrl.SetAccountBlockReleaser(releaser)

	code, body := releaseAccountBlock(t, router, "blocked")
	if code != http.StatusOK || body["released"] != true {
		t.Fatalf("first release = %d %v, want 200 released=true", code, body)
	}
	code, body = releaseAccountBlock(t, router, "blocked")
	if code != http.StatusOK || body["released"] != false {
		t.Fatalf("repeated release = %d %v, want 200 released=false", code, body)
	}
	for _, id := range []string{"missing", "office"} {
		if code, _ := releaseAccountBlock(t, router, id); code != http.StatusNotFound {
			t.Fatalf("release %s status = %d, want 404", id, code)
		}
	}
	if len(releaser.reached) != 2 || releaser.reached[0] != "blocked" || releaser.reached[1] != "blocked" {
		t.Fatalf("release reached %v, want only the named profile", releaser.reached)
	}
	if stored := repo.profiles["blocked"]; stored != blocked || !stored.Enabled || stored.Model != "model-a" {
		t.Fatalf("release changed the profile: %+v", stored)
	}

	releaser.err = errors.New("db is locked")
	if code, _ := releaseAccountBlock(t, router, "sibling"); code != http.StatusInternalServerError {
		t.Fatalf("failed release status = %d, want 500", code)
	}
}
