package coordinator

import "testing"

func TestProposeKind_ProjectScopeRefusesAnOutOfScopeTask(t *testing.T) {
	f := proposeFixture(t)
	f.undo.steps["step-1"] = &UndoStep{WorkflowID: "wf-1"}
	f.undo.steps["manual-step"].WorkflowID = "wf-1"
	mustSave(t, f.svc, f.c.WorkspaceID, f.c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-1"]}}`)
	p := newFakeProjects()
	p.addRepo("repo-a", "a")
	p.addRepo("repo-b", "b")
	f.svc.SetProjectReader(p)
	projectScope(t, f.store, f.c.ID, false, repoEntry("repo-a"))

	p.taskRepos["task-0"] = []string{"repo-b"}
	for _, kind := range []string{ProposalKindResume, ProposalKindMessage, ProposalKindMove} {
		args := map[string]string{
			ProposalKindResume:  `{"task_id":"task-0"}`,
			ProposalKindMessage: `{"task_id":"task-0","text":"hi"}`,
			ProposalKindMove:    `{"task_id":"task-0","step_id":"manual-step"}`,
		}[kind]
		_, _, err := f.propose(kind, args)
		wantField(t, err, "task_id")
	}

	p.taskRepos["task-0"] = []string{"repo-a"}
	if _, _, err := f.propose(ProposalKindResume, `{"task_id":"task-0"}`); err != nil {
		t.Fatalf("a task in a listed project must be accepted: %v", err)
	}
}

func TestApproveKind_ProposalStaysApprovableAfterTheScopeNarrows(t *testing.T) {
	f := newKindsFixture(t)
	p := newFakeProjects()
	p.addRepo("repo-a", "a")
	f.svc.SetProjectReader(p)
	proposal := f.insertMove(t)

	projectScope(t, f.store, f.c.ID, false, repoEntry("repo-a"))
	got, err := f.approve(proposal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ProposalStatusApproved {
		t.Fatalf("approve after the scope narrowed = %q (%s), want approved: approve does not re-check projects",
			got.Status, f.errorOf(t, proposal.ID))
	}
}
