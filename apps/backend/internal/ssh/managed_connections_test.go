package ssh

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/sshconnections"
	cryptossh "golang.org/x/crypto/ssh"
	"testing"
)

type deniedManagedConnections struct{ calls int }

func (d *deniedManagedConnections) OpenConnectionForManagement(context.Context, string, string) (*cryptossh.Client, *lifecycle.SSHTarget, func(), error) {
	d.calls++
	return nil, nil, func() {}, sshconnections.ErrForbidden
}
func (d *deniedManagedConnections) OpenExecutorForManagement(context.Context, string) (*cryptossh.Client, *lifecycle.SSHTarget, func(), error) {
	d.calls++
	return nil, nil, func() {}, sshconnections.ErrForbidden
}
func TestManagedTestSelectorsCannotBypassAuthorityOrFallBack(t *testing.T) {
	h := &Handler{}
	req := TestRequest{ConnectionID: "connection", Stamp: "stamp"}
	if result := h.runTest(context.Background(), req); result.Success || result.Error != sshconnections.ErrDisabled.Error() {
		t.Fatalf("disabled: %+v", result)
	}
	denied := &deniedManagedConnections{}
	h.managedConnections = denied
	if result := h.runTest(context.Background(), req); result.Success || result.Error != sshconnections.ErrForbidden.Error() {
		t.Fatalf("unauthorized: %+v", result)
	}
	req.ConnectionID = ""
	if result := h.runTest(context.Background(), req); result.Success || result.Error != sshconnections.ErrInvalidRoute.Error() || denied.calls != 1 {
		t.Fatalf("empty selector: %+v", result)
	}
}

func TestCutoverRejectsInlineSSHTestPayload(t *testing.T) {
	for _, body := range []string{
		`{"host_alias":"previous"}`,
		`{"ssh_connection_id":"saved","port":0}`,
		`{"ssh_connection_id":"saved","identity_file":""}`,
		`{"ssh_connection_id":"saved","ssh_host":"override"}`,
	} {
		var req TestRequest
		if err := json.Unmarshal([]byte(body), &req); err == nil {
			t.Errorf("accepted retired payload %s", body)
		}
	}
}
