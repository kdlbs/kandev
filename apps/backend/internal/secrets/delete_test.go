package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.11
func TestDeleteWithoutReferenceCheckerFailsClosed(t *testing.T) {
	svc := newSecretsHandlerService(t, nil, nil, nil)
	id := seedGlobalViaService(t, svc, "MY_TOKEN", "private-value")
	rec := doTransferHTTP(t, newSecretsHTTPRouter(t, svc), http.MethodDelete, "/api/v1/secrets/"+id, "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("delete status = %d, want 500 when reference checking is unavailable", rec.Code)
	}
	if _, err := svc.Get(context.Background(), id); err != nil {
		t.Fatalf("secret was removed: %v", err)
	}
}

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.9
func TestDeleteReferencesConflictAndForce(t *testing.T) {
	for _, scope := range []SecretScope{ScopeGlobal, ScopeWorkspace} {
		t.Run(string(scope), func(t *testing.T) {
			svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
			workspaceID, query := "", ""
			if scope == ScopeWorkspace {
				workspaceID, query = "workspace-a", "?workspace_id=workspace-a"
			}
			item := mustCreateViaService(t, svc, "MY_TOKEN", "private-value", scope, workspaceID)
			refs := []Reference{{Kind: "agent_profile", ID: "profile-1", Name: "Claude", Key: "MY_TOKEN"}}
			svc.SetReferenceChecker(func(_ context.Context, id string) ([]Reference, error) {
				if id != item.ID {
					t.Fatalf("checked wrong secret %q", id)
				}
				return refs, nil
			})
			router := newSecretsHTTPRouter(t, svc)
			path := "/api/v1/secrets/" + item.ID + query
			rec := doTransferHTTP(t, router, http.MethodDelete, path, "")
			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
			}
			var conflict struct {
				Code       string      `json:"code"`
				References []Reference `json:"references"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &conflict); err != nil {
				t.Fatal(err)
			}
			if conflict.Code != "secret_in_use" || len(conflict.References) != 1 || conflict.References[0] != refs[0] {
				t.Fatalf("conflict = %+v", conflict)
			}
			if strings.Contains(rec.Body.String(), "private-value") || strings.Contains(rec.Body.String(), item.ID) {
				t.Fatalf("secret leaked: %s", rec.Body)
			}
			get := func() (*Secret, error) {
				if workspaceID != "" {
					return svc.GetWorkspaceSecret(context.Background(), item.ID, workspaceID)
				}
				return svc.Get(context.Background(), item.ID)
			}
			if _, err := get(); err != nil {
				t.Fatalf("secret removed: %v", err)
			}
			separator := "?"
			if query != "" {
				separator = "&"
			}
			for _, force := range []string{"false", "1", "yes"} {
				rec = doTransferHTTP(t, router, http.MethodDelete, path+separator+"force="+force, "")
				if rec.Code != http.StatusConflict {
					t.Fatalf("force=%s status=%d", force, rec.Code)
				}
			}
			rec = doTransferHTTP(t, router, http.MethodDelete, path+separator+"force=true", "")
			if rec.Code != http.StatusNoContent {
				t.Fatalf("forced delete = %d: %s", rec.Code, rec.Body)
			}
			if _, err := get(); !errors.Is(err, ErrNotFound) {
				t.Fatalf("secret remains: %v", err)
			}
		})
	}
}

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.12
func TestListReferencesReturnsConflictsWithoutDeleting(t *testing.T) {
	svc := newSecretsHandlerService(t, nil, nil, nil)
	id := seedGlobalViaService(t, svc, "MY_TOKEN", "private-value")
	refs := []Reference{{Kind: "agent_profile", ID: "profile-1", Name: "Claude", Key: "MY_TOKEN"}}
	svc.SetReferenceChecker(func(_ context.Context, checkedID string) ([]Reference, error) {
		if checkedID != id {
			t.Fatalf("checked wrong secret %q", checkedID)
		}
		return refs, nil
	})

	rec := doTransferHTTP(t, newSecretsHTTPRouter(t, svc), http.MethodGet, "/api/v1/secrets/"+id+"/references", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var response struct {
		References []Reference `json:"references"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.References) != 1 || response.References[0] != refs[0] {
		t.Fatalf("references = %+v", response.References)
	}
	if strings.Contains(rec.Body.String(), "private-value") || strings.Contains(rec.Body.String(), id) {
		t.Fatalf("secret leaked: %s", rec.Body)
	}
	if _, err := svc.Get(context.Background(), id); err != nil {
		t.Fatalf("reference lookup removed secret: %v", err)
	}
}

func TestListReferencesReturnsEmptyArrayWhenUnused(t *testing.T) {
	svc := newSecretsHandlerService(t, nil, nil, nil)
	id := seedGlobalViaService(t, svc, "unused", "private-value")
	svc.SetReferenceChecker(func(context.Context, string) ([]Reference, error) { return nil, nil })

	rec := doTransferHTTP(t, newSecretsHTTPRouter(t, svc), http.MethodGet, "/api/v1/secrets/"+id+"/references", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"references":[]`) {
		t.Fatalf("response = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestListReferencesPreservesScopeAuthorizationAndSanitizesFailures(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	item := mustCreateViaService(t, svc, "workspace token", "private-value", ScopeWorkspace, "workspace-a")
	svc.SetReferenceChecker(func(context.Context, string) ([]Reference, error) {
		return nil, errors.New("private-database-error")
	})
	router := newSecretsHTTPRouter(t, svc)

	missingScope := doTransferHTTP(t, router, http.MethodGet, "/api/v1/secrets/"+item.ID+"/references", "")
	if missingScope.Code != http.StatusNotFound {
		t.Fatalf("missing workspace status = %d, body = %s", missingScope.Code, missingScope.Body)
	}
	failure := doTransferHTTP(t, router, http.MethodGet, "/api/v1/secrets/"+item.ID+"/references?workspace_id=workspace-a", "")
	if failure.Code != http.StatusInternalServerError || strings.Contains(failure.Body.String(), "private-") {
		t.Fatalf("lookup failure = %d, body = %s", failure.Code, failure.Body)
	}
	if _, err := svc.GetWorkspaceSecret(context.Background(), item.ID, "workspace-a"); err != nil {
		t.Fatalf("reference lookup removed secret: %v", err)
	}
}

func TestDeleteUnreferencedSecret(t *testing.T) {
	svc := newSecretsHandlerService(t, nil, nil, nil)
	id := seedGlobalViaService(t, svc, "unused", "value")
	svc.SetReferenceChecker(func(context.Context, string) ([]Reference, error) { return nil, nil })
	if err := svc.Delete(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(context.Background(), id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("secret remains: %v", err)
	}
}

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.11
func TestDeleteLookupFailureIsSanitized(t *testing.T) {
	svc := newSecretsHandlerService(t, nil, nil, nil)
	id := seedGlobalViaService(t, svc, "MY_TOKEN", "private-value")
	svc.SetReferenceChecker(func(context.Context, string) ([]Reference, error) { return nil, errors.New("private-database-error") })
	rec := doTransferHTTP(t, newSecretsHTTPRouter(t, svc), http.MethodDelete, "/api/v1/secrets/"+id, "")
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "private-") {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body)
	}
	if _, err := svc.Get(context.Background(), id); err != nil {
		t.Fatalf("secret removed: %v", err)
	}
}

func TestDeleteForcePreservesAuthorization(t *testing.T) {
	svc := newSecretsHandlerService(t, map[string]bool{"workspace-a": true}, nil, nil)
	item := mustCreateViaService(t, svc, "workspace secret", "value", ScopeWorkspace, "workspace-a")
	svc.SetReferenceChecker(func(context.Context, string) ([]Reference, error) {
		t.Fatal("unauthorized reference lookup")
		return nil, nil
	})
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return ErrWorkspaceAccessDenied })
	for _, force := range []bool{false, true} {
		if err := svc.DeleteWorkspaceSecret(context.Background(), item.ID, "workspace-a", force); !errors.Is(err, ErrWorkspaceAccessDenied) {
			t.Fatalf("force=%v: %v", force, err)
		}
	}
	owner := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
	other := authn.WithIdentity(context.Background(), authn.Identity{UserID: "other", Role: authn.RoleMember})
	global, err := svc.Create(owner, &CreateSecretRequest{Name: "owned", Value: "private-value"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(other, global.ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign delete: %v", err)
	}
	if _, err := svc.Get(owner, global.ID); err != nil {
		t.Fatal(err)
	}
}

func TestWSDeleteConflictAndForce(t *testing.T) {
	svc := newSecretsHandlerService(t, nil, nil, nil)
	id := seedGlobalViaService(t, svc, "MY_TOKEN", "private-value")
	svc.SetReferenceChecker(func(context.Context, string) ([]Reference, error) {
		return []Reference{{Kind: "executor_profile", ID: "exec", Name: "Local", Key: "MY_TOKEN"}}, nil
	})
	h := newWSHandler(t, svc)
	msg := &ws.Message{ID: "1", Action: ws.ActionSecretDelete, Payload: json.RawMessage(`{"id":"` + id + `"}`)}
	resp, err := h.wsDelete(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeWSError(t, resp); got.Code != ws.ErrorCodeConflict {
		t.Fatalf("error = %+v", got)
	}
	if !strings.Contains(string(resp.Payload), `"references"`) {
		t.Fatalf("missing references: %s", resp.Payload)
	}
	msg.Payload = json.RawMessage(`{"id":"` + id + `","force":true}`)
	resp, err = h.wsDelete(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp.Payload), `"success":true`) {
		t.Fatalf("forced result = %s", resp.Payload)
	}
}
