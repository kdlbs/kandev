package plugins

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
)

func TestFilePreviewExternalApprovalHTTP(t *testing.T) {
	router, svc := newTestRouterWithIdentity(t, authn.Identity{UserID: "alice", SessionID: "browser-session", Role: authn.RoleMember})
	svc.SetFilePreviewWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	record := &store.Record{Manifest: manifest.Manifest{ID: "viewer", UI: manifest.UISection{Bundle: "/bundle.js", FileViewers: []manifest.FileViewer{{ID: "stl", Title: "STL", Extensions: []string{"stl"}}}}}, InstallationID: "install", InstalledAt: time.Now(), Status: StatusActive}
	svc.registry.Add(record)
	reads := 0
	svc.SetFilePreviewBackend(func(context.Context, string, string, string) (*FilePreviewSource, error) {
		return &FilePreviewSource{
			WorkspaceID: "workspace", Path: "/tmp/model.stl", External: true, SizeBytes: 4, ExecutionLabel: "remote",
			Read:     func(_ context.Context, w io.Writer) error { reads++; _, err := w.Write([]byte("mesh")); return err },
			Validate: func(context.Context) error { return nil }, Close: func() {},
		}, nil
	})
	base := "/api/plugins/viewer/file-previews"
	query := "?workspace_id=workspace"
	prepared := doRequest(router, http.MethodPost, base+query, `{"session_id":"session","path":"/tmp/model.stl","viewer_id":"stl"}`, jsonHeaders())
	if prepared.Code != 200 {
		t.Fatalf("prepare %d: %s", prepared.Code, prepared.Body.String())
	}
	var result struct {
		ID               string `json:"id"`
		ApprovalRequired bool   `json:"approval_required"`
	}
	if err := json.Unmarshal(prepared.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ID == "" || !result.ApprovalRequired || reads != 0 {
		t.Fatalf("invalid admission: %+v reads %d", result, reads)
	}
	refused := doRequest(router, http.MethodPost, base+"/"+result.ID+"/read"+query, `{"approved":false}`, jsonHeaders())
	if reads != 0 || refused.Code == 200 {
		t.Fatal("external read before approval")
	}
	approved := doRequest(router, http.MethodPost, base+"/"+result.ID+"/read"+query, `{"approved":true}`, jsonHeaders())
	if approved.Code != 200 || approved.Body.String() != "mesh\x00" || reads != 1 {
		t.Fatalf("read %d: %q count %d", approved.Code, approved.Body.String(), reads)
	}
	replay := doRequest(router, http.MethodPost, base+"/"+result.ID+"/read"+query, `{"approved":true}`, jsonHeaders())
	if replay.Code == 200 || reads != 1 {
		t.Fatal("replayed user approval")
	}
}
