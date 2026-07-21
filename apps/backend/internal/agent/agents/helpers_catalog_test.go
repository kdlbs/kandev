package agents

import "testing"

func TestCatalogPermissionSettings_IncludesAgentctlAutoApprove(t *testing.T) {
	catalog := CatalogPermissionSettings(NewClaudeACP())
	setting, ok := catalog[PermissionKeyAutoApprove]
	if !ok {
		t.Fatal("catalog missing auto_approve")
	}
	if setting.ApplyMethod != PermissionApplyMethodAgentctlAutoApprove {
		t.Fatalf("ApplyMethod = %q, want %q", setting.ApplyMethod, PermissionApplyMethodAgentctlAutoApprove)
	}
	if setting.Default {
		t.Fatal("auto_approve must default to false")
	}
}

func TestCatalogPermissionSettings_MergesCursorForce(t *testing.T) {
	catalog := CatalogPermissionSettings(NewCursorACP())
	if _, ok := catalog[PermissionKeyCursorForce]; !ok {
		t.Fatal("missing cursor_force")
	}
	if catalog[PermissionKeyAutoApprove].ApplyMethod != PermissionApplyMethodAgentctlAutoApprove {
		t.Fatal("auto_approve must be agentctl, not cursor --force")
	}
}

func TestCatalogPermissionSettings_ExcludesCodexBridgeCLIFlags(t *testing.T) {
	catalog := CatalogPermissionSettings(NewCodexACP())
	if _, ok := catalog["config_approval_policy_never"]; ok {
		t.Fatal("catalog must not expose stale codex -c config override")
	}
	if _, ok := catalog["config_sandbox_disk_full_read"]; ok {
		t.Fatal("catalog must not expose stale codex sandbox override")
	}
	if _, ok := catalog[PermissionKeyAutoApprove]; !ok {
		t.Fatal("catalog missing universal auto_approve")
	}
}
