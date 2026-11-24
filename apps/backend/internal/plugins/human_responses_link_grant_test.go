package plugins

import (
	"context"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

func linkRequest(code string) pluginsdk.ExternalIdentityLinkRequest {
	return pluginsdk.ExternalIdentityLinkRequest{RequestID: "link-grant", Code: code, Provider: "discord", Subject: "d-grant"}
}

// Linking needs the admin grant. A refused attempt leaves the code unspent, so
// the same code links once a workspace manager grants the installation.
func TestLinkExternalIdentityRequiresTheHumanResponsesGrant(t *testing.T) {
	f := newLinkedResponseFixture(t, []string{"host.v2.write:interactions"})
	code, _, err := f.svc.CreateIdentityLinkCode(linkedPluginID, "alice")
	if err != nil {
		t.Fatalf("CreateIdentityLinkCode: %v", err)
	}

	result, _, err := f.delegation.LinkExternalIdentity(context.Background(), linkRequest(code))
	if err != nil || result.Status != pluginsdk.CommandDenied || result.Reason != string(ApprovalDenyMissingApproval) {
		t.Fatalf("link without the grant = %+v err=%v, want DENIED missing_approval", result, err)
	}
	if links, listErr := f.store.ListIdentityLinks(context.Background(), linkedInstall, ""); listErr != nil || len(links) != 0 {
		t.Fatalf("links after a refused attempt = %+v err=%v, want none", links, listErr)
	}

	if _, err := f.svc.approvalGrant(linkedInstall, linkedWorkspace, 2, f.digest, allLinkedGrants(), "human", "grant", "approval-linked-2"); err != nil {
		t.Fatalf("grant human responses: %v", err)
	}
	result, link, err := f.delegation.LinkExternalIdentity(context.Background(), linkRequest(code))
	if err != nil || result.Status != pluginsdk.CommandApplied || link.UserID != "alice" {
		t.Fatalf("link after the grant = %+v link=%+v err=%v, want the same code to apply", result, link, err)
	}
}

func TestLinkExternalIdentityIsRefusedAfterTheGrantIsRevoked(t *testing.T) {
	f := newLinkedResponseFixture(t, allLinkedGrants())
	if _, err := f.svc.approvalRevoke(linkedInstall, linkedWorkspace, "human", "revoke", "approval-revoke"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	code, _, err := f.svc.CreateIdentityLinkCode(linkedPluginID, "alice")
	if err != nil {
		t.Fatalf("CreateIdentityLinkCode: %v", err)
	}

	result, _, err := f.delegation.LinkExternalIdentity(context.Background(), linkRequest(code))
	if err != nil || result.Status != pluginsdk.CommandDenied {
		t.Fatalf("link after revoke = %+v err=%v, want DENIED", result, err)
	}
}
