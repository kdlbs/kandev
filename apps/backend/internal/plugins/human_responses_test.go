package plugins

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	linkedWorkspace = "workspace-linked"
	otherWorkspace  = "workspace-other"
	linkedPluginID  = "chat-bridge"
	linkedInstall   = "chat-bridge-installation"
)

// linkedResponseFixture is a plugin that declares human_responses, a second
// installation, a durable command store, and a native authorizer that grants
// session.control per user and workspace.
type linkedResponseFixture struct {
	svc          *Service
	host         *pluginHost
	otherHost    *pluginHost
	data         *testDataHost
	store        *state.CommandStore
	digest       string
	activeUsers  map[string]bool
	sessionCtrl  map[string]map[string]bool
	interaction  *taskmodels.Interaction
	version      string
	delegation   pluginsdk.HumanResponseDelegationManager
	interactions pluginsdk.ExactInteractionCommandManager
	connection   *sqlx.DB
}

func newLinkedResponseFixture(t *testing.T, grants []string) *linkedResponseFixture {
	t.Helper()
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open command database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	commandStore, err := state.NewCommandStore(db.NewPool(connection, connection))
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	data := newTestDataHost(readWriteCaps())
	interaction := pendingPermissionInteraction()
	withInteraction(data, interaction)
	data.tasks.tasksByID = map[string]*taskmodels.Task{interaction.TaskID: {ID: interaction.TaskID, WorkspaceID: linkedWorkspace}}
	data.tasks.workspaces = []*taskmodels.Workspace{{ID: linkedWorkspace}}
	resolved := *interaction
	resolved.Status = taskmodels.InteractionStatusApproved
	data.interactions.afterWrite = &resolved

	caps := manifest.Capabilities{APIRead: []string{"interactions", "sessions"}, APIWrite: []string{"interactions", "execution", resourceHumanResponses}}
	record := &pluginstore.Record{Manifest: manifest.Manifest{ID: linkedPluginID, Capabilities: caps}, InstallationID: linkedInstall}
	other := &pluginstore.Record{Manifest: manifest.Manifest{ID: "other-bridge", Capabilities: caps}, InstallationID: "other-installation"}
	registry := NewRegistry()
	registry.Add(record)
	registry.Add(other)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	svc.SetDataSources(data.tasks, data.workflows, data.steps, data.profiles, data.codeStats, data.messages, data.interactions, data.taskWriter)
	svc.SetInteractionResponder(data.responder)
	t.Cleanup(func() { _ = svc.Close() })

	f := &linkedResponseFixture{
		svc: svc, data: data, store: commandStore, interaction: interaction, connection: connection,
		activeUsers: map[string]bool{"alice": true, "bob": true},
		sessionCtrl: map[string]map[string]bool{"alice": {linkedWorkspace: true}},
		version:     digestPublicValue(interactionModelToDTO(interaction)),
	}
	svc.SetLinkedUserResolver(func(_ context.Context, userID string) (LinkedHuman, bool) {
		if !f.activeUsers[userID] {
			return LinkedHuman{}, false
		}
		return LinkedHuman{Identity: authn.Identity{UserID: userID, SessionID: "native-session"}, DisplayName: "Display " + userID}, true
	})
	svc.SetHumanInteractionResponseAuthorizer(func(ctx context.Context, workspaceID string) error {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || !f.sessionCtrl[identity.UserID][workspaceID] {
			return errors.New("forbidden")
		}
		return nil
	})
	f.digest = ManifestCapabilityDigest(record.Manifest)
	if _, err := svc.approvalGrant(record.InstallationID, linkedWorkspace, 1, f.digest, grants, "human", "grant", "approval-linked"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := svc.approvalGrant(other.InstallationID, linkedWorkspace, 1, ManifestCapabilityDigest(other.Manifest), grants, "human", "grant", "approval-other"); err != nil {
		t.Fatalf("grant other installation: %v", err)
	}
	f.host = svc.hostForPlugin(record.ID).(*pluginHost)
	f.otherHost = svc.hostForPlugin(other.ID).(*pluginHost)
	f.delegation, _ = pluginsdk.HostHumanResponseDelegation(f.host)
	f.interactions, _ = pluginsdk.HostExactInteractionCommands(f.host)
	return f
}

func allLinkedGrants() []string {
	return []string{"host.v2.write:interactions", humanResponsesCapability}
}

func (f *linkedResponseFixture) link(t *testing.T, userID, subject string) {
	t.Helper()
	code, _, err := f.svc.CreateIdentityLinkCode(linkedPluginID, userID)
	if err != nil {
		t.Fatalf("CreateIdentityLinkCode: %v", err)
	}
	result, _, err := f.delegation.LinkExternalIdentity(context.Background(), pluginsdk.ExternalIdentityLinkRequest{
		RequestID: "link-" + subject, Code: code, Provider: "discord", Subject: subject, DisplayName: "discord " + subject,
	})
	if err != nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("link %s = %+v err=%v", subject, result, err)
	}
}

func (f *linkedResponseFixture) receiptRequest(subject, optionID string) pluginsdk.HumanResponseReceiptRequest {
	return pluginsdk.HumanResponseReceiptRequest{
		RequestID: "receipt-" + subject, WorkspaceID: linkedWorkspace, Provider: "discord", Subject: subject,
		Kind: pluginsdk.HumanResponseKindPermission, TargetID: f.interaction.ID, ExpectedResourceVersion: f.version,
		OptionID: optionID, ApprovalRevision: 1, ManifestDigest: f.digest,
	}
}

func (f *linkedResponseFixture) respond(manager pluginsdk.ExactInteractionCommandManager, requestID, receiptID, optionID string) (*pluginsdk.CommandResult, *pluginsdk.Interaction) {
	result, interaction, _ := manager.RespondPermission(context.Background(), pluginsdk.ExactPermissionResponse{
		RequestID: requestID, WorkspaceID: linkedWorkspace, InteractionID: f.interaction.ID, ExpectedResourceVersion: f.version,
		OptionID: optionID, HumanResponseReceiptID: receiptID, ApprovalRevision: 1, ManifestDigest: f.digest,
	})
	return result, interaction
}

func TestLinkedHumanResponseActsAsTheLinkedUserAndIsAttributed(t *testing.T) {
	f := newLinkedResponseFixture(t, allLinkedGrants())
	f.link(t, "alice", "discord-alice")

	result, receipt, err := f.delegation.RequestHumanResponseReceipt(context.Background(), f.receiptRequest("discord-alice", "allow"))
	if err != nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("receipt = %+v err=%v", result, err)
	}
	wantActor := pluginsdk.InteractionActor{UserID: "alice", DisplayName: "Display alice", Via: "discord"}
	if receipt.Actor != wantActor || receipt.ResourceVersion != f.version {
		t.Fatalf("receipt = %+v, want actor %+v at the observed version", receipt, wantActor)
	}

	applied, resolved := f.respond(f.interactions, "respond-1", receipt.ID, "allow")
	if applied.Status != pluginsdk.CommandApplied || f.data.responder.permission == nil {
		t.Fatalf("respond = %+v responder=%+v", applied, f.data.responder.permission)
	}
	if got := f.data.responder.permission; got.actor != "alice" || !got.in.LinkedUser {
		t.Fatalf("responder ran as %q linked=%v, want alice through the linked path", got.actor, got.in.LinkedUser)
	}
	if resolved == nil || resolved.ResolvedBy == nil || *resolved.ResolvedBy != wantActor {
		t.Fatalf("resolved interaction attribution = %+v, want %+v", resolved, wantActor)
	}
	read, err := f.host.Interactions().Get(context.Background(), f.interaction.ID)
	if err != nil || read.ResolvedBy == nil || *read.ResolvedBy != wantActor {
		t.Fatalf("Interactions().Get attribution = %+v err=%v", read, err)
	}
	audit, err := f.store.ConsumedDelegatedResponse(context.Background(), f.interaction.ID)
	if err != nil || audit.UserID != "alice" || audit.Subject != "discord-alice" || audit.InstallationID != linkedInstall {
		t.Fatalf("audit = %+v err=%v", audit, err)
	}

	// Replaying the receipt never responds twice: the same command replays
	// its outcome, and a new command with the spent receipt is refused.
	replayed, _ := f.respond(f.interactions, "respond-1", receipt.ID, "allow")
	if replayed.Status != pluginsdk.CommandAlreadyApplied || f.data.responder.permissionCalls != 1 {
		t.Fatalf("replay = %+v calls=%d, want ALREADY_APPLIED without a second response", replayed, f.data.responder.permissionCalls)
	}
	reused, _ := f.respond(f.interactions, "respond-2", receipt.ID, "allow")
	if reused.Status == pluginsdk.CommandApplied || reused.Status == pluginsdk.CommandAlreadyApplied || f.data.responder.permissionCalls != 1 {
		t.Fatalf("reused receipt = %+v calls=%d, want a refusal without a second response", reused, f.data.responder.permissionCalls)
	}
}

//nolint:funlen // One table names every denial the linked flow must enforce.
func TestLinkedHumanResponseDenials(t *testing.T) {
	cases := []struct {
		name   string
		grants []string
		setup  func(*testing.T, *linkedResponseFixture) (*pluginsdk.CommandResult, *pluginsdk.CommandResult)
		reason string
	}{
		{
			name: "grant narrowed to exclude human responses", grants: allLinkedGrants(),
			setup: func(t *testing.T, f *linkedResponseFixture) (*pluginsdk.CommandResult, *pluginsdk.CommandResult) {
				f.link(t, "alice", "d-1")
				if _, err := f.svc.approvalGrant(linkedInstall, linkedWorkspace, 2, f.digest, []string{"host.v2.write:interactions"}, "human", "narrow", "narrow-linked"); err != nil {
					t.Fatalf("narrow grant: %v", err)
				}
				result, _, _ := f.delegation.RequestHumanResponseReceipt(context.Background(), f.receiptRequest("d-1", "allow"))
				return result, nil
			},
		},
		{
			name: "revoked grant", grants: allLinkedGrants(), reason: string(ApprovalDenyRevokedApproval),
			setup: func(t *testing.T, f *linkedResponseFixture) (*pluginsdk.CommandResult, *pluginsdk.CommandResult) {
				f.link(t, "alice", "d-1")
				if _, err := f.svc.approvalRevoke(linkedInstall, linkedWorkspace, "human", "revoke", "revoke-linked"); err != nil {
					t.Fatalf("revoke: %v", err)
				}
				result, _, _ := f.delegation.RequestHumanResponseReceipt(context.Background(), f.receiptRequest("d-1", "allow"))
				return result, nil
			},
		},
		{
			name: "unlinked subject", grants: allLinkedGrants(), reason: "external_identity_not_linked",
			setup: func(_ *testing.T, f *linkedResponseFixture) (*pluginsdk.CommandResult, *pluginsdk.CommandResult) {
				result, _, _ := f.delegation.RequestHumanResponseReceipt(context.Background(), f.receiptRequest("d-unknown", "allow"))
				return result, nil
			},
		},
		{
			name: "user without session control", grants: allLinkedGrants(), reason: "linked_user_not_authorized",
			setup: func(t *testing.T, f *linkedResponseFixture) (*pluginsdk.CommandResult, *pluginsdk.CommandResult) {
				f.link(t, "bob", "d-bob")
				result, _, _ := f.delegation.RequestHumanResponseReceipt(context.Background(), f.receiptRequest("d-bob", "allow"))
				return result, nil
			},
		},
		{
			name: "cross-workspace interaction", grants: allLinkedGrants(), reason: "interaction_not_found",
			setup: func(t *testing.T, f *linkedResponseFixture) (*pluginsdk.CommandResult, *pluginsdk.CommandResult) {
				f.link(t, "alice", "d-1")
				f.data.tasks.tasksByID[f.interaction.TaskID].WorkspaceID = otherWorkspace
				result, _, _ := f.delegation.RequestHumanResponseReceipt(context.Background(), f.receiptRequest("d-1", "allow"))
				return result, nil
			},
		},
		{
			name: "payload mismatch", grants: allLinkedGrants(), reason: "human_response_receipt_invalid_or_used",
			setup: func(t *testing.T, f *linkedResponseFixture) (*pluginsdk.CommandResult, *pluginsdk.CommandResult) {
				f.link(t, "alice", "d-1")
				minted, receipt, _ := f.delegation.RequestHumanResponseReceipt(context.Background(), f.receiptRequest("d-1", "allow"))
				result, _ := f.respond(f.interactions, "respond-mismatch", receipt.ID, "deny")
				return minted, result
			},
		},
		{
			name: "link revoked before consumption", grants: allLinkedGrants(), reason: "external_identity_not_linked",
			setup: func(t *testing.T, f *linkedResponseFixture) (*pluginsdk.CommandResult, *pluginsdk.CommandResult) {
				f.link(t, "alice", "d-1")
				minted, receipt, _ := f.delegation.RequestHumanResponseReceipt(context.Background(), f.receiptRequest("d-1", "allow"))
				links, _ := f.svc.ListIdentityLinks(context.Background(), linkedPluginID, "alice", false)
				if err := f.svc.RevokeIdentityLink(context.Background(), linkedPluginID, links[0].ID, "alice", false); err != nil {
					t.Fatalf("revoke link: %v", err)
				}
				result, _ := f.respond(f.interactions, "respond-revoked", receipt.ID, "allow")
				return minted, result
			},
		},
		{
			name: "session control lost before consumption", grants: allLinkedGrants(), reason: "linked_user_not_authorized",
			setup: func(t *testing.T, f *linkedResponseFixture) (*pluginsdk.CommandResult, *pluginsdk.CommandResult) {
				f.link(t, "alice", "d-1")
				minted, receipt, _ := f.delegation.RequestHumanResponseReceipt(context.Background(), f.receiptRequest("d-1", "allow"))
				f.sessionCtrl["alice"][linkedWorkspace] = false
				result, _ := f.respond(f.interactions, "respond-lost-control", receipt.ID, "allow")
				return minted, result
			},
		},
		{
			name: "receipt presented by another installation", grants: allLinkedGrants(), reason: "human_response_receipt_invalid_or_used",
			setup: func(t *testing.T, f *linkedResponseFixture) (*pluginsdk.CommandResult, *pluginsdk.CommandResult) {
				f.link(t, "alice", "d-1")
				minted, receipt, _ := f.delegation.RequestHumanResponseReceipt(context.Background(), f.receiptRequest("d-1", "allow"))
				other, _ := pluginsdk.HostExactInteractionCommands(f.otherHost)
				otherDigest := ManifestCapabilityDigest(f.otherHost.service.installedRecordByInstallationID("other-installation").Manifest)
				result, _, _ := other.RespondPermission(context.Background(), pluginsdk.ExactPermissionResponse{
					RequestID: "respond-other", WorkspaceID: linkedWorkspace, InteractionID: f.interaction.ID,
					ExpectedResourceVersion: f.version, OptionID: "allow", HumanResponseReceiptID: receipt.ID,
					ApprovalRevision: 1, ManifestDigest: otherDigest,
				})
				return minted, result
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLinkedResponseFixture(t, tc.grants)
			first, second := tc.setup(t, f)
			denied := first
			if second != nil {
				if first == nil || first.Status != pluginsdk.CommandApplied {
					t.Fatalf("receipt = %+v, want it minted before the consumption check", first)
				}
				denied = second
			}
			if denied == nil || denied.Status == pluginsdk.CommandApplied || (tc.reason != "" && denied.Reason != tc.reason) {
				t.Fatalf("result = %+v, want a refusal with reason %q", denied, tc.reason)
			}
			if f.data.responder.writeCalled {
				t.Fatal("a refused linked response reached the native responder")
			}
		})
	}
}

func TestLinkExternalIdentityRequiresDeclarationAndAFreshCode(t *testing.T) {
	f := newLinkedResponseFixture(t, allLinkedGrants())
	code, _, err := f.svc.CreateIdentityLinkCode(linkedPluginID, "alice")
	if err != nil {
		t.Fatalf("CreateIdentityLinkCode: %v", err)
	}
	other, _ := pluginsdk.HostHumanResponseDelegation(f.otherHost)
	request := pluginsdk.ExternalIdentityLinkRequest{RequestID: "link", Code: code, Provider: "discord", Subject: "d-1"}
	if result, _, _ := other.LinkExternalIdentity(context.Background(), request); result.Status != pluginsdk.CommandDenied {
		t.Fatalf("foreign installation redeem = %+v, want DENIED", result)
	}
	// A code shown to the wrong installation is spent.
	if result, _, _ := f.delegation.LinkExternalIdentity(context.Background(), request); result.Status != pluginsdk.CommandDenied {
		t.Fatalf("redeem after a foreign attempt = %+v, want DENIED", result)
	}

	code, _, _ = f.svc.CreateIdentityLinkCode(linkedPluginID, "alice")
	request.Code = " " + code[:4] + code[5:9] + "-" + code[10:] + " "
	result, link, err := f.delegation.LinkExternalIdentity(context.Background(), request)
	if err != nil || result.Status != pluginsdk.CommandApplied || link.UserID != "alice" || link.UserDisplayName != "Display alice" {
		t.Fatalf("redeem = %+v link=%+v err=%v", result, link, err)
	}
	if result, _, _ := f.delegation.LinkExternalIdentity(context.Background(), request); result.Status != pluginsdk.CommandDenied {
		t.Fatalf("second redeem = %+v, want DENIED", result)
	}

	expired, _, _ := f.svc.CreateIdentityLinkCode(linkedPluginID, "bob")
	f.svc.identityLinkCodes.now = func() time.Time { return time.Now().Add(identityLinkCodeTTL + time.Minute) }
	request.Code = expired
	if result, _, _ := f.delegation.LinkExternalIdentity(context.Background(), request); result.Status != pluginsdk.CommandDenied {
		t.Fatalf("expired redeem = %+v, want DENIED", result)
	}

	undeclared := newTestDataHost(readWriteCaps()).host
	manager := humanResponseDelegationManager{host: undeclared}
	if result, _, _ := manager.LinkExternalIdentity(context.Background(), request); result.Status != pluginsdk.CommandDenied || result.Reason != string(ApprovalDenyUndeclaredCapability) {
		t.Fatalf("undeclared link = %+v, want DENIED undeclared_capability", result)
	}
	if _, _, err := f.svc.CreateIdentityLinkCode("p1", "alice"); err == nil {
		t.Fatal("link code for a plugin without the declaration was issued")
	}
}

func TestIdentityLinkManagementScopesUsersAndAdmins(t *testing.T) {
	f := newLinkedResponseFixture(t, allLinkedGrants())
	f.link(t, "alice", "d-alice")
	f.link(t, "bob", "d-bob")
	ctx := context.Background()

	own, err := f.svc.ListIdentityLinks(ctx, linkedPluginID, "alice", false)
	if err != nil || len(own) != 1 || own[0].Subject != "d-alice" {
		t.Fatalf("alice's links = %+v err=%v", own, err)
	}
	all, err := f.svc.ListIdentityLinks(ctx, linkedPluginID, "alice", true)
	if err != nil || len(all) != 2 {
		t.Fatalf("admin links = %+v err=%v", all, err)
	}
	bobLink := all[1]
	if bobLink.UserID != "bob" {
		bobLink = all[0]
	}
	if err := f.svc.RevokeIdentityLink(ctx, linkedPluginID, bobLink.ID, "alice", false); !errors.Is(err, ErrIdentityLinkNotFound) {
		t.Fatalf("alice revoking bob's link err = %v, want not found", err)
	}
	if err := f.svc.RevokeIdentityLink(ctx, linkedPluginID, bobLink.ID, "admin", true); err != nil {
		t.Fatalf("admin revoke: %v", err)
	}
	if links, _ := f.svc.ListIdentityLinks(ctx, linkedPluginID, "", true); len(links) != 1 {
		t.Fatalf("links after admin revoke = %+v, want one", links)
	}
	if err := f.svc.deleteInstallationIdentityLinks(ctx, linkedInstall); err != nil {
		t.Fatalf("uninstall cleanup: %v", err)
	}
	if links, _ := f.svc.ListIdentityLinks(ctx, linkedPluginID, "", true); len(links) != 0 {
		t.Fatalf("links after uninstall cleanup = %+v, want none", links)
	}
}

func TestApproveStepActsAsTheLinkedUserOnce(t *testing.T) {
	f := newLinkedResponseFixture(t, append(allLinkedGrants(), "host.v2.read:sessions", "host.v2.write:execution"))
	controller := &fakeExactExecutionController{stepAwaiting: true, stepVersion: "sha256:step-1"}
	f.svc.SetExactExecutionController(controller)
	f.link(t, "alice", "d-1")
	execution, _ := pluginsdk.HostExecutionCommands(f.host)
	ctx := context.Background()

	approval, err := execution.GetStepApproval(ctx, pluginsdk.ExactStepApprovalQuery{RequestID: "read-step", WorkspaceID: linkedWorkspace, SessionID: "session-1"})
	if err != nil || !approval.Awaiting || approval.ToStepID != "done" {
		t.Fatalf("GetStepApproval = %+v err=%v", approval, err)
	}
	receiptRequest := pluginsdk.HumanResponseReceiptRequest{
		RequestID: "receipt-step", WorkspaceID: linkedWorkspace, Provider: "discord", Subject: "d-1",
		Kind: pluginsdk.HumanResponseKindStepApproval, TargetID: "session-1", ExpectedResourceVersion: approval.ResourceVersion,
		ApprovalRevision: 1, ManifestDigest: f.digest,
	}
	stale := receiptRequest
	stale.ExpectedResourceVersion = "sha256:old"
	if result, _, _ := f.delegation.RequestHumanResponseReceipt(ctx, stale); result.Status != pluginsdk.CommandConflict {
		t.Fatalf("stale step receipt = %+v, want CONFLICT", result)
	}
	minted, receipt, err := f.delegation.RequestHumanResponseReceipt(ctx, receiptRequest)
	if err != nil || minted.Status != pluginsdk.CommandApplied {
		t.Fatalf("step receipt = %+v err=%v", minted, err)
	}
	command := pluginsdk.ExactStepApprovalCommand{
		RequestID: "approve-1", WorkspaceID: linkedWorkspace, SessionID: "session-1",
		ExpectedResourceVersion: approval.ResourceVersion, IdempotencyKey: "approve-1",
		HumanResponseReceiptID: receipt.ID, ApprovalRevision: 1, ManifestDigest: f.digest,
	}
	result, approved, err := execution.ApproveStep(ctx, command)
	if err != nil || result.Status != pluginsdk.CommandApplied || approved.ToStepID != "done" || controller.approveActorID != "alice" {
		t.Fatalf("ApproveStep = %+v approved=%+v actor=%q err=%v", result, approved, controller.approveActorID, err)
	}
	replayed, replayedResult, _ := execution.ApproveStep(ctx, command)
	if replayed.Status != pluginsdk.CommandAlreadyApplied || replayedResult.ToStepID != "done" || controller.approveCalls != 1 {
		t.Fatalf("replay = %+v %+v calls=%d, want ALREADY_APPLIED without a second move", replayed, replayedResult, controller.approveCalls)
	}
	if _, err := f.store.ConsumedDelegatedResponse(ctx, "session-1"); err != nil {
		t.Fatalf("step approval audit: %v", err)
	}

	// Nothing awaits approval any more: a fresh receipt is refused up front.
	if result, _, _ := f.delegation.RequestHumanResponseReceipt(ctx, receiptRequest); result.Status != pluginsdk.CommandConflict || result.Reason != reasonStepNotAwaitingApproval {
		t.Fatalf("receipt after approval = %+v, want CONFLICT step_not_awaiting_approval", result)
	}

	controller.stepErr = statusUnimplemented()
	if _, err := execution.GetStepApproval(ctx, pluginsdk.ExactStepApprovalQuery{RequestID: "read-office", WorkspaceID: linkedWorkspace, SessionID: "session-1"}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("office stage read err = %v, want Unimplemented", err)
	}
}

func TestApproveStepWithoutAwaitingIsConflict(t *testing.T) {
	f := newLinkedResponseFixture(t, append(allLinkedGrants(), "host.v2.read:sessions", "host.v2.write:execution"))
	controller := &fakeExactExecutionController{stepAwaiting: true, stepVersion: "sha256:step-1"}
	f.svc.SetExactExecutionController(controller)
	f.link(t, "alice", "d-1")
	execution, _ := pluginsdk.HostExecutionCommands(f.host)
	ctx := context.Background()
	_, receipt, _ := f.delegation.RequestHumanResponseReceipt(ctx, pluginsdk.HumanResponseReceiptRequest{
		RequestID: "receipt-step", WorkspaceID: linkedWorkspace, Provider: "discord", Subject: "d-1",
		Kind: pluginsdk.HumanResponseKindStepApproval, TargetID: "session-1", ExpectedResourceVersion: "sha256:step-1",
		ApprovalRevision: 1, ManifestDigest: f.digest,
	})
	// Approved elsewhere (for example the native button) after the receipt.
	controller.stepAwaiting = false
	result, _, err := execution.ApproveStep(ctx, pluginsdk.ExactStepApprovalCommand{
		RequestID: "approve-late", WorkspaceID: linkedWorkspace, SessionID: "session-1",
		ExpectedResourceVersion: "sha256:step-1", IdempotencyKey: "approve-late",
		HumanResponseReceiptID: receipt.ID, ApprovalRevision: 1, ManifestDigest: f.digest,
	})
	if err != nil || result.Status != pluginsdk.CommandConflict || result.Reason != reasonStepNotAwaitingApproval || controller.approveCalls != 0 {
		t.Fatalf("late approval = %+v calls=%d err=%v, want CONFLICT step_not_awaiting_approval", result, controller.approveCalls, err)
	}
}

func statusUnimplemented() error {
	return status.Error(codes.Unimplemented, "office approval stage")
}
