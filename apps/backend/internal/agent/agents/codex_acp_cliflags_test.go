package agents

import "testing"

func TestCodexACP_NoLegacyAdapterCLIFlags(t *testing.T) {
	if _, ok := codexACPPermSettings["config_approval_policy_never"]; ok {
		t.Fatal("codex-acp must not expose the old -c approval_policy flag")
	}
	if _, ok := codexACPPermSettings["config_sandbox_disk_full_read"]; ok {
		t.Fatal("codex-acp must not expose the old -c sandbox_permissions flag")
	}
}
