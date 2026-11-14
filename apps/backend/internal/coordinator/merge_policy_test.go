package coordinator

import (
	"errors"
	"testing"
)

func yes(string) bool { return true }
func no(string) bool  { return false }

func TestPolicyDefaultDenied(t *testing.T) {
	if storedPolicy(nil, "r1") != RepoDenied {
		t.Fatal("missing policy must be denied")
	}
	if storedPolicy(map[string]RepoPolicyRow{"r1": {Policy: "bogus"}}, "r1") != RepoDenied {
		t.Fatal("unreadable policy must be denied")
	}
}

func TestPolicyCASConflict(t *testing.T) {
	req := []RepoPolicyChange{{RepositoryID: "r1", Policy: RepoRequiresApproval}}
	_, err := planRepoPolicies(nil, req, "u1", true, 3, 4, no)
	var c *MergeConflictError
	if !errors.As(err, &c) || c.Code != codeRevisionConflict {
		t.Fatalf("err = %v", err)
	}
	_, err = planRepoPolicies(nil, req, "u1", false, 0, 4, no)
	var se *SettingsError
	if !errors.As(err, &se) || se.Code != codeRevisionRequired {
		t.Fatalf("err = %v", err)
	}
}

func TestUnchangedSaveNoRevision(t *testing.T) {
	stored := map[string]RepoPolicyRow{"r1": {RepositoryID: "r1", Policy: RepoRequiresApproval}}
	plan, err := planRepoPolicies(stored, []RepoPolicyChange{{RepositoryID: "r1", Policy: RepoRequiresApproval}, {RepositoryID: "r2", Policy: RepoDenied}}, "u1", false, 0, 9, no)
	if err != nil || plan.changed() || len(plan.Audits) != 0 {
		t.Fatalf("plan = %+v err = %v", plan, err)
	}
}

func TestAutomaticRecordsPrincipalAndRevision(t *testing.T) {
	plan, err := planRepoPolicies(nil, []RepoPolicyChange{{RepositoryID: "r1", Policy: RepoAutomatic}}, "u1", true, 4, 4, no)
	if err != nil || len(plan.Rows) != 1 {
		t.Fatalf("plan = %+v err = %v", plan, err)
	}
	r := plan.Rows[0]
	if r.SelectedBy != "u1" || r.AutomaticSinceRevision != 5 || plan.Audits[0].Revision != 5 || plan.Audits[0].Before != "denied" {
		t.Fatalf("row = %+v audit = %+v", r, plan.Audits[0])
	}
	down, _ := planRepoPolicies(map[string]RepoPolicyRow{"r1": r}, []RepoPolicyChange{{RepositoryID: "r1", Policy: RepoRequiresApproval}}, "u2", true, 5, 5, yes)
	if down.Rows[0].SelectedBy != "" || down.Rows[0].AutomaticSinceRevision != 0 {
		t.Fatalf("leaving automatic kept the principal: %+v", down.Rows[0])
	}
}

func TestPrincipalReplacement(t *testing.T) {
	stored := map[string]RepoPolicyRow{"r1": {RepositoryID: "r1", Policy: RepoAutomatic, SelectedBy: "u1", AutomaticSinceRevision: 2}}
	req := []RepoPolicyChange{{RepositoryID: "r1", Policy: RepoAutomatic, ReplacePrincipal: true}}
	_, err := planRepoPolicies(stored, req, "u2", true, 6, 6, yes)
	var c *MergeConflictError
	if !errors.As(err, &c) || c.Code != codePrincipalValid {
		t.Fatalf("valid principal replaced: %v", err)
	}
	_, err = planRepoPolicies(stored, req, "u2", true, 5, 6, yes)
	if !errors.As(err, &c) || c.Code != codeRevisionConflict {
		t.Fatalf("stale revision must win over principal_valid: %v", err)
	}
	_, err = planRepoPolicies(stored, req, "u2", false, 0, 6, no)
	var se *SettingsError
	if !errors.As(err, &se) || se.Code != codeRevisionRequired {
		t.Fatalf("replacement needs the revision: %v", err)
	}
	plan, err := planRepoPolicies(stored, req, "u2", true, 6, 6, no)
	if err != nil || plan.Rows[0].SelectedBy != "u2" || plan.Rows[0].AutomaticSinceRevision != 2 || plan.Audits[0].Kind != auditPrincipalReplaced {
		t.Fatalf("plan = %+v err = %v", plan, err)
	}
	if a := plan.Audits[0]; a.Before != "u1" || a.After != "u2" {
		t.Fatalf("audit must record the previous principal: %+v", a)
	}
	// A listed automatic entry without the marker is unchanged.
	plan, _ = planRepoPolicies(stored, []RepoPolicyChange{{RepositoryID: "r1", Policy: RepoAutomatic}}, "u2", false, 0, 6, no)
	if plan.changed() {
		t.Fatalf("plain automatic entry changed state: %+v", plan)
	}
}

func TestPartialMapAtomicOnError(t *testing.T) {
	stored := map[string]RepoPolicyRow{"r2": {RepositoryID: "r2", Policy: RepoAutomatic, SelectedBy: "u1", AutomaticSinceRevision: 1}}
	req := []RepoPolicyChange{
		{RepositoryID: "r1", Policy: RepoRequiresApproval},
		{RepositoryID: "r2", Policy: RepoAutomatic, ReplacePrincipal: true},
	}
	plan, err := planRepoPolicies(stored, req, "u2", true, 3, 3, yes)
	if err == nil || plan.changed() {
		t.Fatalf("a failing entry must store nothing: %+v %v", plan, err)
	}
}

func TestDuplicateListedRepositoryRefused(t *testing.T) {
	req := []RepoPolicyChange{{RepositoryID: "r1", Policy: RepoDenied}, {RepositoryID: "r1", Policy: RepoAutomatic}}
	if _, err := planRepoPolicies(nil, req, "u", true, 1, 1, no); err == nil {
		t.Fatal("duplicate entry accepted")
	}
}
