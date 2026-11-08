package coordinator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func (f *notesFixture) router() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &Handlers{service: f.svc, logger: newTestLogger(f.t)}
	registerNoteRoutes(r.Group("/api/v1/workspaces/:id"), h)
	return r
}

func (f *notesFixture) do(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/workspaces/"+f.c.WorkspaceID+"/coordinators/"+f.c.ID+"/notes"+path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestNoteRoutesStatuses(t *testing.T) {
	f := newNotesFixture(t)
	r := f.router()
	rec := f.do(r, http.MethodPost, "", `{"type":"user","text":"hello"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	var v NoteView
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	if rec := f.do(r, http.MethodPost, "", `{"type":"zzz","text":"hello"}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), NoteInvalidType) {
		t.Fatalf("bad type = %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(r, http.MethodPut, "/"+v.ID, `{"type":"user","text":"x","citations":[],"version":9}`); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"reason":"version"`) {
		t.Fatalf("conflict = %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(r, http.MethodPost, "", strings.Repeat("x", maxNoteBodyBytes+1)); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), NoteInvalidRequest) {
		t.Fatalf("oversize = %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(r, http.MethodGet, "/"+v.ID, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"versions"`) {
		t.Fatalf("detail = %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(r, http.MethodGet, "/nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", rec.Code)
	}
	if rec := f.do(r, http.MethodPost, "/"+v.ID+"/delete", `{"version":1}`); rec.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body)
	}
	f.svc.notes = false
	if rec := f.do(r, http.MethodGet, "", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("flag off = %d", rec.Code)
	}
}

func TestNoteRowsDeletedWithCoordinator(t *testing.T) {
	f := newNotesFixture(t)
	v := f.create("one")
	if err := f.store.DeleteCoordinator(t.Context(), f.c.WorkspaceID, f.c.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"coordinator_notes", "coordinator_note_versions"} {
		var n int
		if err := f.store.db.Get(&n, `SELECT COUNT(*) FROM `+table); err != nil || n != 0 {
			t.Fatalf("%s rows = %d %v (note %s)", table, n, err, v.ID)
		}
	}
}

func TestNoteSchemaReplay(t *testing.T) {
	f := newNotesFixture(t)
	if err := f.store.migrateNotes(); err != nil {
		t.Fatalf("replay: %v", err)
	}
}
