package coordinator

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/authz"
	svcpkg "github.com/kandev/kandev/internal/task/service"
)

// manageDenied authorizes reads and refuses the manager scope, as a reader.
type manageDenied struct{}

func (manageDenied) AuthorizeWorkspaceScope(_ context.Context, _ string, scope authz.Scope) error {
	if scope == authz.ScopeWorkspaceManage {
		return svcpkg.ErrForbidden
	}
	return nil
}

type fakeHealth struct{}

func (fakeHealth) Health(context.Context, *Coordinator) (LearningHealth, error) {
	return LearningHealth{State: "waiting", Condition: "autonomy_off"}, nil
}

func learningEnv(t *testing.T) (*Handlers, *phase3Env) {
	t.Helper()
	h, env := phase3Handlers(t, true)
	env.svc.phase31 = true
	env.svc.SetLearningHealth(fakeHealth{})
	return h, env
}

func dreamParams(cid, dreamID, itemID string) gin.Params {
	p := workspaceParams(cid)
	p = append(p, gin.Param{Key: "dreamId", Value: dreamID})
	if itemID != "" {
		p = append(p, gin.Param{Key: "itemId", Value: itemID})
	}
	return p
}

func seedReport(t *testing.T, env *phase3Env, cid string) {
	t.Helper()
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	d := Dream{ID: "d1", CoordinatorID: cid, WindowStart: at.Add(-24 * time.Hour), WindowEnd: at, InputHash: "h", StartedAt: at}
	st := env.svc.store
	if ok, err := st.InsertRunningDream(context.Background(), d); err != nil || !ok {
		t.Fatal(ok, err)
	}
	d.Status, d.TurnIDs, d.Considered = DreamOK, []string{"a", "b"}, []string{"raise it"}
	items := []DreamItem{{ID: "i1", Kind: "note_add", Text: "x", CitedTurnIDs: []string{"a", "b"}, Gate: "pass"}}
	if ok, err := st.FinishDream(context.Background(), d, items, at); err != nil || !ok {
		t.Fatal(ok, err)
	}
}

func TestLearningRoutes_ReadSwitchAndPublish(t *testing.T) {
	h, env := learningEnv(t)
	rec := runHandler(h.httpGetLearning, http.MethodGet, "/x", "", workspaceParams(env.c.ID))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"shadow_dream":false`) || !strings.Contains(rec.Body.String(), "autonomy_off") {
		t.Fatalf("get = %d %s", rec.Code, rec.Body.String())
	}
	before, _ := env.svc.store.GetCoordinator(context.Background(), testWorkspaceID, env.c.ID)
	rec = runHandler(h.httpPutLearning, http.MethodPut, "/x", `{"shadow_dream":true}`, workspaceParams(env.c.ID))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"shadow_dream":true`) {
		t.Fatalf("put = %d %s", rec.Code, rec.Body.String())
	}
	after, _ := env.svc.store.GetCoordinator(context.Background(), testWorkspaceID, env.c.ID)
	if after.PolicyRevision != before.PolicyRevision || after.ConfigRevision != before.ConfigRevision {
		t.Fatal("the switch must not change the policy or config revision")
	}
	for _, body := range []string{``, `{}`, `{"shadow_dream":"yes"}`} {
		if rec := runHandler(h.httpPutLearning, http.MethodPut, "/x", body, workspaceParams(env.c.ID)); rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q = %d, want 400", body, rec.Code)
		}
	}
}

func TestLearningRoutes_FlagOffAndForeignAre404(t *testing.T) {
	h, env := learningEnv(t)
	env.svc.phase31 = false
	for name, fn := range map[string]gin.HandlerFunc{"learning": h.httpGetLearning, "dreams": h.httpListDreams} {
		if rec := runHandler(fn, http.MethodGet, "/x", "", dreamParams(env.c.ID, "d1", "")); rec.Code != http.StatusNotFound {
			t.Fatalf("%s with the flag off = %d", name, rec.Code)
		}
	}
	env.svc.phase31 = true
	if rec := runHandler(h.httpGetLearning, http.MethodGet, "/x", "", workspaceParams("nope")); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown coordinator = %d", rec.Code)
	}
}

func TestDreamRoutes_ListDetailRatingAndCoordinatorScoping(t *testing.T) {
	h, env := learningEnv(t)
	seedReport(t, env, env.c.ID)

	rec := runHandler(h.httpListDreams, http.MethodGet, "/x", "", workspaceParams(env.c.ID))
	var page DreamsPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Dreams) != 1 || page.Dreams[0].ItemCount != 1 || page.Dreams[0].TurnCount != 2 {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	if rec := runHandler(h.httpListDreams, http.MethodGet, "/x?before=garbage", "", workspaceParams(env.c.ID)); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor = %d", rec.Code)
	}

	rec = runHandler(h.httpGetDream, http.MethodGet, "/x", "", dreamParams(env.c.ID, "d1", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "raise it") || !strings.Contains(rec.Body.String(), `"i1"`) {
		t.Fatalf("detail = %d %s", rec.Code, rec.Body.String())
	}

	// A dream of another coordinator is a 404 whatever its id.
	other := newTestCoordinator(t, env.svc.store, testWorkspaceID)
	if rec := runHandler(h.httpGetDream, http.MethodGet, "/x", "", dreamParams(other.ID, "d1", "")); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign dream = %d", rec.Code)
	}
	if rec := runHandler(h.httpPutDreamRating, http.MethodPut, "/x", `{"rating":"useful"}`, dreamParams(other.ID, "d1", "i1")); rec.Code != http.StatusNotFound {
		t.Fatalf("rating through another coordinator = %d", rec.Code)
	}
	if rec := runHandler(h.httpPutDreamRating, http.MethodPut, "/x", `{"rating":"useful"}`, dreamParams(env.c.ID, "d1", "nope")); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown item = %d", rec.Code)
	}
	if rec := runHandler(h.httpPutDreamRating, http.MethodPut, "/x", `{"rating":"great"}`, dreamParams(env.c.ID, "d1", "i1")); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad rating = %d", rec.Code)
	}
	if rec := runHandler(h.httpPutDreamRating, http.MethodPut, "/x", `{"rating":"harmful"}`, dreamParams(env.c.ID, "d1", "i1")); rec.Code != http.StatusNoContent {
		t.Fatalf("rating = %d", rec.Code)
	}
	rec = runHandler(h.httpGetDream, http.MethodGet, "/x", "", dreamParams(env.c.ID, "d1", ""))
	if !strings.Contains(rec.Body.String(), `"rating":"harmful"`) {
		t.Fatalf("rating not shown: %s", rec.Body.String())
	}
}

func TestLearningRoutes_ReaderIsReadOnly(t *testing.T) {
	h, env := learningEnv(t)
	seedReport(t, env, env.c.ID)
	env.svc.authz = manageDenied{}
	if rec := runHandler(h.httpGetLearning, http.MethodGet, "/x", "", workspaceParams(env.c.ID)); rec.Code != http.StatusOK {
		t.Fatalf("reader get = %d", rec.Code)
	}
	if rec := runHandler(h.httpGetDream, http.MethodGet, "/x", "", dreamParams(env.c.ID, "d1", "")); rec.Code != http.StatusOK {
		t.Fatalf("reader detail = %d", rec.Code)
	}
	if rec := runHandler(h.httpPutLearning, http.MethodPut, "/x", `{"shadow_dream":true}`, workspaceParams(env.c.ID)); rec.Code != http.StatusForbidden {
		t.Fatalf("reader switch = %d, want 403", rec.Code)
	}
	if rec := runHandler(h.httpPutDreamRating, http.MethodPut, "/x", `{"rating":"useful"}`, dreamParams(env.c.ID, "d1", "i1")); rec.Code != http.StatusForbidden {
		t.Fatalf("reader rating = %d, want 403", rec.Code)
	}
	if on, _ := env.svc.store.ShadowDreamEnabled(context.Background(), env.c.ID); on {
		t.Fatal("a refused switch changed the stored value")
	}
}

func TestListDreams_PagesOfTwentyWithCursor(t *testing.T) {
	h, env := learningEnv(t)
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 23; i++ {
		d := Dream{ID: "p" + string(rune('a'+i)), CoordinatorID: env.c.ID, InputHash: "h", StartedAt: at.Add(time.Duration(i) * time.Hour), WindowStart: at, WindowEnd: at}
		if ok, err := env.svc.store.InsertRunningDream(context.Background(), d); err != nil || !ok {
			t.Fatal(ok, err)
		}
		d.Status = DreamClean
		if ok, err := env.svc.store.FinishDream(context.Background(), d, nil, at); err != nil || !ok {
			t.Fatal(ok, err)
		}
	}
	var first, second DreamsPage
	rec := runHandler(h.httpListDreams, http.MethodGet, "/x", "", workspaceParams(env.c.ID))
	_ = json.Unmarshal(rec.Body.Bytes(), &first)
	if len(first.Dreams) != 20 || first.NextBefore == "" {
		t.Fatalf("first page = %d next=%q", len(first.Dreams), first.NextBefore)
	}
	rec = runHandler(h.httpListDreams, http.MethodGet, "/x?before="+first.NextBefore, "", workspaceParams(env.c.ID))
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	if len(second.Dreams) != 3 || second.NextBefore != "" {
		t.Fatalf("second page = %d next=%q", len(second.Dreams), second.NextBefore)
	}
}
