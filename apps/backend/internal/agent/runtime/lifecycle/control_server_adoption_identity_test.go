package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

// TestIdentityMatchesRecordedServerRequiresRecordedIdentity pins the closed
// side of the identity proof. A record with no recorded server identity cannot
// prove which process it describes, and an installation-wide value such as the
// home directory proves nothing beyond a shared installation, which every
// control server on the machine satisfies. An unprovable identity is refused
// rather than adopted.
func TestIdentityMatchesRecordedServerRequiresRecordedIdentity(t *testing.T) {
	identity := &agentctl.IdentityInfo{ServerIdentity: "live-nonce"}

	if identityMatchesRecordedServer(identity, &models.ControlServerRecord{}) {
		t.Fatal("a record carrying no server identity was treated as proven")
	}

	matching := &models.ControlServerRecord{ServerIdentity: "live-nonce"}
	if !identityMatchesRecordedServer(identity, matching) {
		t.Fatal("a record whose server identity matches the live one was refused")
	}

	other := &models.ControlServerRecord{ServerIdentity: "some-other-nonce"}
	if identityMatchesRecordedServer(identity, other) {
		t.Fatal("a record whose server identity differs from the live one was accepted")
	}
}

// TestIdentityMatchesRecordedServerRequiresAdvertisedIdentity covers the other
// half: a live server that advertises no identity is equally unprovable, so it
// cannot satisfy a record that names one.
func TestIdentityMatchesRecordedServerRequiresAdvertisedIdentity(t *testing.T) {
	record := &models.ControlServerRecord{ServerIdentity: "recorded-nonce"}

	if identityMatchesRecordedServer(&agentctl.IdentityInfo{}, record) {
		t.Fatal("a server advertising no identity was treated as proven")
	}
}
