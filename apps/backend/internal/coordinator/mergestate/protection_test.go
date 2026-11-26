package mergestate

import "testing"

func TestProtectionUnknownOnFailure(t *testing.T) {
	got := GitHubProtection(ClassicRead{State: ClassicFailed}, RulesetsRead{OK: false})
	if got != UnknownProtection() {
		t.Fatalf("failed reads = %+v", got)
	}
	// A failed source never turns a not-enforced reading on the other into one.
	got = GitHubProtection(ClassicRead{State: ClassicFound}, RulesetsRead{OK: false})
	if got.Pipeline != UnknownEnf {
		t.Fatalf("one failed source must stay unknown: %+v", got)
	}
}

func TestGitHubRulesetsAndClassic404(t *testing.T) {
	req := ProtectionRequirements{Pipeline: true}
	// A ruleset enforces even when classic protection cannot be read.
	got := GitHubProtection(ClassicRead{State: ClassicAbsent}, RulesetsRead{OK: true, Requires: req})
	if got.Pipeline != Enforced || got.Approvals != UnknownEnf {
		t.Fatalf("ruleset over unconfirmed 404 = %+v", got)
	}
	// A classic 404 is "no rule" only with confirmed admin access.
	got = GitHubProtection(ClassicRead{State: ClassicAbsent}, RulesetsRead{OK: true})
	if got.Pipeline != UnknownEnf {
		t.Fatalf("unconfirmed 404 must be unknown: %+v", got)
	}
	got = GitHubProtection(ClassicRead{State: ClassicAbsent, AdminConfirmed: true}, RulesetsRead{OK: true})
	if got != (Protection{Pipeline: NotEnforced, Approvals: NotEnforced, Threads: NotEnforced}) {
		t.Fatalf("confirmed absence = %+v", got)
	}
	got = GitHubProtection(ClassicRead{State: ClassicFound, Requires: ProtectionRequirements{Approvals: true, Threads: true}}, RulesetsRead{OK: true, Requires: req})
	if got != (Protection{Pipeline: Enforced, Approvals: Enforced, Threads: Enforced}) {
		t.Fatalf("both sources = %+v", got)
	}
}
