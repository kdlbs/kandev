package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
)

const capabilityApprovalTestWorkspace = "workspace-approval-test"

func TestCapabilityApprovalHandlersGrantNarrowAuditAndRevoke(t *testing.T) {
	router, svc := newTestRouterWithIdentity(t, authn.Identity{UserID: "workspace-owner", Role: authn.RoleMember})
	svc.SetCapabilityApprovalWorkspaceAuthorizer(func(ctx context.Context, workspaceID string) error {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || identity.UserID != "workspace-owner" || workspaceID != capabilityApprovalTestWorkspace {
			return fmt.Errorf("workspace management denied")
		}
		return nil
	})
	installed, err := svc.Install(t.Context(), capabilityApprovalTestPackage(t, "kandev-plugin-approval-test"))
	if err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	endpoint := "/api/plugins/" + installed.ID + "/capability-approvals"
	workspaceQuery := "?workspace_id=" + capabilityApprovalTestWorkspace

	initial := doRequest(router, http.MethodGet, endpoint+workspaceQuery, "", nil)
	if initial.Code != http.StatusOK {
		t.Fatalf("initial GET status = %d, body=%s", initial.Code, initial.Body.String())
	}
	var contextResponse struct {
		InstallationID        string                       `json:"installation_id"`
		WorkspaceID           string                       `json:"workspace_id"`
		ManifestDigest        string                       `json:"manifest_digest"`
		DeclaredCapabilityIDs []string                     `json:"declared_capability_ids"`
		Approval              *CapabilityApprovalDTO       `json:"approval"`
		AuditEvents           []CapabilityApprovalEventDTO `json:"audit_events"`
	}
	if err := json.Unmarshal(initial.Body.Bytes(), &contextResponse); err != nil {
		t.Fatalf("decode initial GET: %v", err)
	}
	if contextResponse.InstallationID != installed.InstallationID || contextResponse.WorkspaceID != capabilityApprovalTestWorkspace {
		t.Fatalf("approval context identity = %#v", contextResponse)
	}
	if contextResponse.ManifestDigest != ManifestCapabilityDigest(installed.Manifest) || len(contextResponse.DeclaredCapabilityIDs) != 2 {
		t.Fatalf("approval context manifest = %#v", contextResponse)
	}
	if contextResponse.Approval != nil || len(contextResponse.AuditEvents) != 0 {
		t.Fatalf("initial approval context = %#v, want no approval or history", contextResponse)
	}

	grant := doRequest(router, http.MethodPut, endpoint, `{"workspace_id":"workspace-approval-test","expected_revision":0,"manifest_digest":"`+contextResponse.ManifestDigest+`","capability_ids":["host.v2.read:tasks","host.v2.write:tasks"],"reason":"Allow task coordination","audit_id":"grant-1"}`, jsonHeaders())
	if grant.Code != http.StatusOK {
		t.Fatalf("grant status = %d, body=%s", grant.Code, grant.Body.String())
	}
	var granted CapabilityApprovalDTO
	if err := json.Unmarshal(grant.Body.Bytes(), &granted); err != nil {
		t.Fatalf("decode grant: %v", err)
	}
	if granted.Revision != 1 || granted.HumanActor != "workspace-owner" || len(granted.CapabilityIDs) != 2 {
		t.Fatalf("grant result = %#v", granted)
	}

	stale := doRequest(router, http.MethodPut, endpoint, `{"workspace_id":"workspace-approval-test","expected_revision":0,"manifest_digest":"`+contextResponse.ManifestDigest+`","capability_ids":["host.v2.read:tasks"],"reason":"Stale grant","audit_id":"stale-grant"}`, jsonHeaders())
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale grant status = %d, body=%s, want 409", stale.Code, stale.Body.String())
	}

	narrow := doRequest(router, http.MethodPut, endpoint, `{"workspace_id":"workspace-approval-test","expected_revision":1,"manifest_digest":"`+contextResponse.ManifestDigest+`","capability_ids":["host.v2.read:tasks"],"reason":"Read only now","audit_id":"narrow-1"}`, jsonHeaders())
	if narrow.Code != http.StatusOK {
		t.Fatalf("narrow status = %d, body=%s", narrow.Code, narrow.Body.String())
	}

	audit := doRequest(router, http.MethodGet, endpoint+workspaceQuery, "", nil)
	if audit.Code != http.StatusOK {
		t.Fatalf("audit GET status = %d, body=%s", audit.Code, audit.Body.String())
	}
	if err := json.Unmarshal(audit.Body.Bytes(), &contextResponse); err != nil {
		t.Fatalf("decode audit GET: %v", err)
	}
	if len(contextResponse.AuditEvents) != 2 || contextResponse.AuditEvents[1].Type != string(CapabilityApprovalEventNarrow) {
		t.Fatalf("audit events = %#v, want grant followed by narrow", contextResponse.AuditEvents)
	}
	if decision := svc.AuthorizeCapability(installed.InstallationID, capabilityApprovalTestWorkspace, "host.v2.write:tasks", 2, "request", "method"); decision.Allowed {
		t.Fatal("narrowed write capability remained authorized")
	}

	revoke := doRequest(router, http.MethodDelete, endpoint, `{"workspace_id":"workspace-approval-test","expected_revision":2,"reason":"Stop task coordination","audit_id":"revoke-1"}`, jsonHeaders())
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, body=%s", revoke.Code, revoke.Body.String())
	}
	var revoked CapabilityApprovalDTO
	if err := json.Unmarshal(revoke.Body.Bytes(), &revoked); err != nil {
		t.Fatalf("decode revoke: %v", err)
	}
	if revoked.State != string(ApprovalStateRevoked) || revoked.Revision != 3 {
		t.Fatalf("revoke result = %#v", revoked)
	}
	if decision := svc.AuthorizeCapability(installed.InstallationID, capabilityApprovalTestWorkspace, "host.v2.read:tasks", 3, "request", "method"); decision.Allowed {
		t.Fatal("revoked capability remained authorized")
	}
}

func TestCapabilityApprovalHandlersRequireWorkspaceManager(t *testing.T) {
	router, svc := newTestRouterWithIdentity(t, authn.Identity{UserID: "workspace-member", Role: authn.RoleMember})
	svc.SetCapabilityApprovalWorkspaceAuthorizer(func(context.Context, string) error {
		return fmt.Errorf("insufficient workspace permissions")
	})
	installed, err := svc.Install(t.Context(), capabilityApprovalTestPackage(t, "kandev-plugin-approval-member"))
	if err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	endpoint := "/api/plugins/" + installed.ID + "/capability-approvals"
	query := "?workspace_id=" + capabilityApprovalTestWorkspace
	read := doRequest(router, http.MethodGet, endpoint+query, "", nil)
	if read.Code != http.StatusForbidden {
		t.Fatalf("member GET status = %d, body=%s, want 403", read.Code, read.Body.String())
	}
	write := doRequest(router, http.MethodPut, endpoint, `{"workspace_id":"workspace-approval-test","expected_revision":0,"manifest_digest":"`+ManifestCapabilityDigest(installed.Manifest)+`","capability_ids":["host.v2.read:tasks"],"reason":"grant","audit_id":"member-grant"}`, jsonHeaders())
	if write.Code != http.StatusForbidden {
		t.Fatalf("member PUT status = %d, body=%s, want 403", write.Code, write.Body.String())
	}
	if _, found, err := svc.GetCapabilityApproval(installed.InstallationID, capabilityApprovalTestWorkspace); err != nil || found {
		t.Fatalf("approval after denied member request = found:%v err:%v", found, err)
	}
}

func TestCapabilityApprovalHandlersSurfaceManifestUpgradeReview(t *testing.T) {
	router, svc := newTestRouterWithIdentity(t, authn.Identity{UserID: "workspace-owner", Role: authn.RoleMember})
	svc.SetCapabilityApprovalWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	first, err := svc.Install(t.Context(), capabilityApprovalTestPackageWithScopes(
		t, "kandev-plugin-approval-upgrade", "1.0.0", []string{"tasks"}, []string{"tasks"},
	))
	if err != nil {
		t.Fatalf("install initial plugin: %v", err)
	}
	if _, err := svc.GrantCapabilityApproval(
		first.InstallationID, capabilityApprovalTestWorkspace, 1, ManifestCapabilityDigest(first.Manifest),
		[]string{"host.v2.read:tasks", "host.v2.write:tasks"}, "workspace-owner", "Initial access", "upgrade-grant",
	); err != nil {
		t.Fatalf("grant initial approval: %v", err)
	}

	upgraded, err := svc.Install(t.Context(), capabilityApprovalTestPackageWithScopes(
		t, "kandev-plugin-approval-upgrade", "1.1.0", []string{"tasks", "sessions"}, []string{"tasks"},
	))
	if err != nil {
		t.Fatalf("upgrade plugin: %v", err)
	}
	if upgraded.InstallationID != first.InstallationID {
		t.Fatalf("installation identity changed on upgrade: %q -> %q", first.InstallationID, upgraded.InstallationID)
	}

	response := doRequest(router, http.MethodGet,
		"/api/plugins/"+first.ID+"/capability-approvals?workspace_id="+capabilityApprovalTestWorkspace, "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("upgraded approval GET status = %d, body=%s", response.Code, response.Body.String())
	}
	var contextResponse capabilityApprovalContextDTO
	if err := json.Unmarshal(response.Body.Bytes(), &contextResponse); err != nil {
		t.Fatalf("decode upgraded approval context: %v", err)
	}
	if !contextResponse.RequiresReview {
		t.Fatalf("upgrade review state = %#v, want review required", contextResponse)
	}
	if contextResponse.Approval == nil || contextResponse.Approval.Revision != 2 {
		t.Fatalf("approval after upgrade = %#v, want revision 2", contextResponse.Approval)
	}
	if got := contextResponse.AuditEvents[len(contextResponse.AuditEvents)-1].Type; got != string(CapabilityApprovalEventUpgradeReview) {
		t.Fatalf("latest audit event = %q, want upgrade review", got)
	}
}

func capabilityApprovalTestPackage(t *testing.T, id string) *bytes.Buffer {
	t.Helper()
	return capabilityApprovalTestPackageWithScopes(t, id, "1.0.0", []string{"tasks"}, []string{"tasks"})
}

func capabilityApprovalTestPackageWithScopes(t *testing.T, id, version string, reads, writes []string) *bytes.Buffer {
	t.Helper()
	platform := runtime.GOOS + "-" + runtime.GOARCH
	readResources, err := json.Marshal(reads)
	if err != nil {
		t.Fatalf("encode read resources: %v", err)
	}
	writeResources, err := json.Marshal(writes)
	if err != nil {
		t.Fatalf("encode write resources: %v", err)
	}
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: %s
display_name: Approval Test Plugin
capabilities:
  api_read: %s
  api_write: %s
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, version, readResources, writeResources, platform)
	var buffer bytes.Buffer
	if err := pkgtartest.WritePackage(&buffer, map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}); err != nil {
		t.Fatalf("build plugin package: %v", err)
	}
	return &buffer
}

func jsonHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json"}
}
