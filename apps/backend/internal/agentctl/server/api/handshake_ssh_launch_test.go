package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/common/sshlaunch"
)

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.18
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestHandshakeSSHLaunchReceiptBindsSelfAndOwner(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("bound SSH launch proof is issued only for Linux remotes")
	}
	generation, err := sshlaunch.NewGeneration()
	if err != nil {
		t.Fatalf("NewGeneration: %v", err)
	}
	binding := sshlaunch.Binding{
		Generation: generation,
		Owner: sshlaunch.Owner{
			TaskID:               "task-1",
			SessionID:            "session-1",
			ControllerInstanceID: "instance-1",
			TaskDir:              "/home/kandev/.kandev/tasks/task-1",
			SessionDir:           "/home/kandev/.kandev/tasks/task-1/sessions/session-1",
			Target: sshlaunch.TargetBinding{
				Host:            "runner.example.test",
				Port:            "22",
				User:            "kandev",
				HostFingerprint: "SHA256:test",
			},
		},
	}
	cfg := &config.Config{
		AuthToken:        "self-generated-token",
		BootstrapNonce:   "launch-nonce",
		SSHLaunchBinding: &binding,
	}
	cs := setupHandshakeServer(cfg)

	body, _ := json.Marshal(map[string]string{"nonce": "launch-nonce"})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/handshake", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	cs.router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("handshake status = %d: %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Token  string             `json:"token"`
		Launch *sshlaunch.Receipt `json:"launch"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode handshake response: %v", err)
	}
	if response.Launch == nil {
		t.Fatal("handshake returned no launch receipt for a bound SSH launch")
	}
	if err := response.Launch.ValidateBinding(binding); err != nil {
		t.Fatalf("returned receipt does not describe the requested launch: %v", err)
	}
	if response.Launch.Process.PID != os.Getpid() {
		t.Fatalf("receipt pid = %d, want the responding controller pid %d", response.Launch.Process.PID, os.Getpid())
	}
	if response.Launch.UID != os.Geteuid() {
		t.Fatalf("receipt uid = %d, want %d", response.Launch.UID, os.Geteuid())
	}
	state, err := processidentity.Inspect(response.Launch.Process)
	if err != nil {
		t.Fatalf("Inspect(self) = %v", err)
	}
	if state != processidentity.StateAlive {
		t.Fatalf("self-captured identity state = %q, want alive", state)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.18
func TestHandshakeWithoutBindingOmitsLaunchReceipt(t *testing.T) {
	cfg := &config.Config{
		AuthToken:      "self-generated-token",
		BootstrapNonce: "plain-nonce",
	}
	cs := setupHandshakeServer(cfg)

	body, _ := json.Marshal(map[string]string{"nonce": "plain-nonce"})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/handshake", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	cs.router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("handshake status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode handshake response: %v", err)
	}
	if _, present := response["launch"]; present {
		t.Fatal("handshake returned launch proof for an unbound launch")
	}
}
