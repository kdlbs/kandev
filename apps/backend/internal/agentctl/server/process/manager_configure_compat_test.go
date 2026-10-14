package process

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-004.1, .2, .3
// approval_policy was sent on every configure call and read by nothing. The
// receiving DTO still accepts it so an older backend configuring a newer
// agentctl keeps working, but nothing derives behavior from it: permission
// auto-approval travels on CreateInstanceRequest.AutoApprovePermissions.
func TestConfigureIgnoresRetiredApprovalPolicy(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:                t.TempDir(),
		AutoApprovePermissions: true,
	}, newTestLogger(t))
	t.Cleanup(mgr.stopWorkspaceTrackers)

	if err := mgr.Configure("echo", []string{"echo"}, true, map[string]string{"A": "b"}, "", nil, false); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	if !mgr.cfg.AutoApprovePermissions {
		t.Fatal("auto-approval must keep travelling on its own carrier")
	}
	if mgr.cfg.AgentCommand != "echo" {
		t.Fatalf("command = %q, want echo", mgr.cfg.AgentCommand)
	}
	env := environmentMap(mgr.cfg.AgentEnv)
	if env["A"] != "b" {
		t.Fatalf("env = %#v, want the configured entry", env)
	}
}

// The configure surface must expose no permission knob other than the one
// carrier, so a future reader cannot mistake a stored string for policy.
func TestInstanceConfigHasNoApprovalPolicyField(t *testing.T) {
	cfg := config.InstanceConfig{}
	rendered := strings.ToLower(structFieldNames(reflect.TypeOf(cfg)))
	if strings.Contains(rendered, "approvalpolicy") {
		t.Fatal("InstanceConfig still carries ApprovalPolicy; it is read by nothing")
	}
}

func structFieldNames(t reflect.Type) string {
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		names = append(names, t.Field(i).Name)
	}
	return strings.Join(names, ",")
}
