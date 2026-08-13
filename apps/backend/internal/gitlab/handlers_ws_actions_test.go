package gitlab

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// actionRecordingClient captures the arguments each write handler forwards to
// the GitLab client, so the tests can assert the exact upstream call rather
// than only the response envelope. Any method left unset delegates to the
// embedded MockClient.
type actionRecordingClient struct {
	*MockClient

	labelCalls    []labelCall
	assigneeCalls []assigneeCall
	noteCalls     []noteCall
	resolveCalls  []resolveCall
	mergeMethods  *ProjectMergeMethods
	mergeErr      error
}

type labelCall struct {
	Project string
	IID     int
	Labels  []string
}

type assigneeCall struct {
	Project string
	IID     int
	IDs     []int
}

type noteCall struct {
	Project      string
	IID          int
	DiscussionID string
	Body         string
}

type resolveCall struct {
	Project      string
	IID          int
	DiscussionID string
}

func (c *actionRecordingClient) SetMRLabels(_ context.Context, project string, iid int, labels []string) error {
	c.labelCalls = append(c.labelCalls, labelCall{Project: project, IID: iid, Labels: labels})
	return nil
}

func (c *actionRecordingClient) SetMRAssignees(_ context.Context, project string, iid int, ids []int) error {
	c.assigneeCalls = append(c.assigneeCalls, assigneeCall{Project: project, IID: iid, IDs: ids})
	return nil
}

func (c *actionRecordingClient) CreateMRDiscussionNote(_ context.Context, project string, iid int, discussionID, body string) (*MRNote, error) {
	c.noteCalls = append(c.noteCalls, noteCall{Project: project, IID: iid, DiscussionID: discussionID, Body: body})
	return &MRNote{ID: 99, Body: body}, nil
}

func (c *actionRecordingClient) ResolveMRDiscussion(_ context.Context, project string, iid int, discussionID string) error {
	c.resolveCalls = append(c.resolveCalls, resolveCall{Project: project, IID: iid, DiscussionID: discussionID})
	return nil
}

func (c *actionRecordingClient) GetProjectMergeMethods(ctx context.Context, project string) (*ProjectMergeMethods, error) {
	if c.mergeErr != nil {
		return nil, c.mergeErr
	}
	if c.mergeMethods != nil {
		return c.mergeMethods, nil
	}
	return c.MockClient.GetProjectMergeMethods(ctx, project)
}

func newActionFixture(t *testing.T) (*Service, *actionRecordingClient, *logger.Logger) {
	t.Helper()
	client := &actionRecordingClient{MockClient: NewMockClient(DefaultHost)}
	client.SetUser("alice")
	svc, _, log := newWSFixture(t, client)
	return svc, client, log
}

// projectAndIIDValidationCases is shared by every handler whose guard is the
// same "project and iid required" triple.
func projectAndIIDValidationCases(t *testing.T, action string, handler ws.HandlerFunc) {
	t.Helper()
	cases := map[string]*ws.Message{
		"missing project": wsRequest(t, action, map[string]interface{}{"iid": 1}),
		"missing iid":     wsRequest(t, action, map[string]interface{}{"project": "g/p"}),
		"malformed":       wsRawRequest(action, `{"iid":"one"}`),
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			resp, err := handler(context.Background(), msg)
			wsErr(t, resp, err, action, ws.ErrorCodeBadRequest, "project and iid required")
		})
	}
}

// --- Merge ---

func TestWSMergeMRMergesAndReturnsMR(t *testing.T) {
	svc, client, log := newActionFixture(t)
	iid := client.SeedMR("group/p", &MR{Title: "feat: x", State: "opened"})

	resp, err := wsMergeMR(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRMerge, map[string]interface{}{
			"project": "group/p", "iid": iid, "method": "merge",
		}))

	var mr MR
	wsOK(t, resp, err, ws.ActionGitLabMRMerge, &mr)
	if mr.State != "merged" {
		t.Errorf("state = %q, want merged", mr.State)
	}
}

// TestWSMergeMRSurfacesDisallowedMethod pins that method validation happens
// before the merge call: the project below allows only merge commits, so a
// fast-forward request must fail rather than silently merging some other way.
func TestWSMergeMRSurfacesDisallowedMethod(t *testing.T) {
	svc, client, log := newActionFixture(t)
	client.mergeMethods = &ProjectMergeMethods{Merge: true}
	iid := client.SeedMR("group/p", &MR{Title: "feat: x", State: "opened"})

	resp, err := wsMergeMR(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRMerge, map[string]interface{}{
			"project": "group/p", "iid": iid, "method": "ff",
		}))

	payload := wsErr(t, resp, err, ws.ActionGitLabMRMerge, ws.ErrorCodeInternalError, "")
	if payload.Message != "project does not allow fast-forward merge" {
		t.Errorf("message = %q, want the fast-forward rejection", payload.Message)
	}
	mr, getErr := client.GetMR(context.Background(), "group/p", iid)
	if getErr != nil {
		t.Fatalf("reload MR: %v", getErr)
	}
	if mr.State == "merged" {
		t.Error("MR was merged despite the disallowed method")
	}
}

func TestWSMergeMRValidatesProjectAndIID(t *testing.T) {
	svc, _, log := newActionFixture(t)
	projectAndIIDValidationCases(t, ws.ActionGitLabMRMerge, wsMergeMR(svc, log))
}

// --- Approve / unapprove ---

func TestWSApproveMRRecordsApproval(t *testing.T) {
	svc, client, log := newActionFixture(t)
	iid := client.SeedMR("group/p", &MR{Title: "feat: x", State: "opened"})

	resp, err := wsApproveMR(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRApprove, map[string]interface{}{"project": "group/p", "iid": iid}))

	var body map[string]bool
	wsOK(t, resp, err, ws.ActionGitLabMRApprove, &body)
	if !body["approved"] {
		t.Errorf("payload = %v, want {\"approved\":true}", body)
	}
	approvals, err := client.ListMRApprovals(context.Background(), "group/p", iid)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(approvals) != 1 || approvals[0].Username != "alice" {
		t.Errorf("approvals = %+v, want alice's single approval", approvals)
	}
}

func TestWSApproveMRSurfacesUpstreamFailure(t *testing.T) {
	svc, _, log := newActionFixture(t)

	resp, err := wsApproveMR(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRApprove, map[string]interface{}{"project": "group/p", "iid": 404}))

	wsErr(t, resp, err, ws.ActionGitLabMRApprove, ws.ErrorCodeInternalError, "")
}

func TestWSApproveMRValidatesProjectAndIID(t *testing.T) {
	svc, _, log := newActionFixture(t)
	projectAndIIDValidationCases(t, ws.ActionGitLabMRApprove, wsApproveMR(svc, log))
}

func TestWSUnapproveMRRemovesApproval(t *testing.T) {
	svc, client, log := newActionFixture(t)
	iid := client.SeedMR("group/p", &MR{Title: "feat: x", State: "opened"})
	client.SeedApprovals("group/p", iid, []MRApproval{{Username: "alice"}}, 1)

	resp, err := wsUnapproveMR(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRUnapprove, map[string]interface{}{"project": "group/p", "iid": iid}))

	var body map[string]bool
	wsOK(t, resp, err, ws.ActionGitLabMRUnapprove, &body)
	if !body["unapproved"] {
		t.Errorf("payload = %v, want {\"unapproved\":true}", body)
	}
	approvals, err := client.ListMRApprovals(context.Background(), "group/p", iid)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(approvals) != 0 {
		t.Errorf("approvals = %+v, want alice's approval revoked", approvals)
	}
}

func TestWSUnapproveMRValidatesProjectAndIID(t *testing.T) {
	svc, _, log := newActionFixture(t)
	projectAndIIDValidationCases(t, ws.ActionGitLabMRUnapprove, wsUnapproveMR(svc, log))
}

// --- Labels / assignees ---

func TestWSSetMRLabelsForwardsExactLabels(t *testing.T) {
	svc, client, log := newActionFixture(t)

	resp, err := wsSetMRLabels(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRSetLabels, map[string]interface{}{
			"project": "group/p", "iid": 5, "labels": []string{"bug", "p1"},
		}))

	var body map[string]bool
	wsOK(t, resp, err, ws.ActionGitLabMRSetLabels, &body)
	if !body["updated"] {
		t.Errorf("payload = %v, want {\"updated\":true}", body)
	}
	if len(client.labelCalls) != 1 {
		t.Fatalf("label calls = %d, want exactly 1", len(client.labelCalls))
	}
	call := client.labelCalls[0]
	if call.Project != "group/p" || call.IID != 5 {
		t.Errorf("call target = %s!%d, want group/p!5", call.Project, call.IID)
	}
	if len(call.Labels) != 2 || call.Labels[0] != "bug" || call.Labels[1] != "p1" {
		t.Errorf("labels = %v, want [bug p1] in order", call.Labels)
	}
}

// TestWSSetMRLabelsForwardsEmptyListToClearLabels guards the "remove every
// label" case: an empty list must still reach the client, because dropping the
// call would silently leave the old labels in place.
func TestWSSetMRLabelsForwardsEmptyListToClearLabels(t *testing.T) {
	svc, client, log := newActionFixture(t)

	resp, err := wsSetMRLabels(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRSetLabels, map[string]interface{}{
			"project": "group/p", "iid": 5, "labels": []string{},
		}))

	wsOK(t, resp, err, ws.ActionGitLabMRSetLabels, nil)
	if len(client.labelCalls) != 1 {
		t.Fatalf("label calls = %d, want 1 even for an empty list", len(client.labelCalls))
	}
	if len(client.labelCalls[0].Labels) != 0 {
		t.Errorf("labels = %v, want an empty list", client.labelCalls[0].Labels)
	}
}

func TestWSSetMRLabelsValidatesProjectAndIID(t *testing.T) {
	svc, _, log := newActionFixture(t)
	projectAndIIDValidationCases(t, ws.ActionGitLabMRSetLabels, wsSetMRLabels(svc, log))
}

func TestWSSetMRAssigneesForwardsExactIDs(t *testing.T) {
	svc, client, log := newActionFixture(t)

	resp, err := wsSetMRAssignees(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRSetAssignees, map[string]interface{}{
			"project": "group/p", "iid": 6, "assignee_ids": []int{7, 8},
		}))

	var body map[string]bool
	wsOK(t, resp, err, ws.ActionGitLabMRSetAssignees, &body)
	if !body["updated"] {
		t.Errorf("payload = %v, want {\"updated\":true}", body)
	}
	if len(client.assigneeCalls) != 1 {
		t.Fatalf("assignee calls = %d, want exactly 1", len(client.assigneeCalls))
	}
	call := client.assigneeCalls[0]
	if call.Project != "group/p" || call.IID != 6 {
		t.Errorf("call target = %s!%d, want group/p!6", call.Project, call.IID)
	}
	if len(call.IDs) != 2 || call.IDs[0] != 7 || call.IDs[1] != 8 {
		t.Errorf("assignee ids = %v, want [7 8]", call.IDs)
	}
}

func TestWSSetMRAssigneesValidatesProjectAndIID(t *testing.T) {
	svc, _, log := newActionFixture(t)
	projectAndIIDValidationCases(t, ws.ActionGitLabMRSetAssignees, wsSetMRAssignees(svc, log))
}

// --- Discussions ---

func TestWSNewDiscussionNoteForwardsBody(t *testing.T) {
	svc, client, log := newActionFixture(t)

	resp, err := wsNewDiscussionNote(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRDiscussionNew, map[string]interface{}{
			"project": "group/p", "iid": 9, "discussion_id": "disc-1", "body": "looks good",
		}))

	var note MRNote
	wsOK(t, resp, err, ws.ActionGitLabMRDiscussionNew, &note)
	if note.Body != "looks good" {
		t.Errorf("note body = %q, want the posted body echoed back", note.Body)
	}
	if len(client.noteCalls) != 1 {
		t.Fatalf("note calls = %d, want exactly 1", len(client.noteCalls))
	}
	if client.noteCalls[0] != (noteCall{Project: "group/p", IID: 9, DiscussionID: "disc-1", Body: "looks good"}) {
		t.Errorf("note call = %+v, want the exact request arguments", client.noteCalls[0])
	}
}

func TestWSNewDiscussionNoteRequiresProjectIIDAndDiscussion(t *testing.T) {
	svc, _, log := newActionFixture(t)
	handler := wsNewDiscussionNote(svc, log)

	cases := map[string]interface{}{
		"missing project":    map[string]interface{}{"iid": 1, "discussion_id": "d", "body": "b"},
		"missing iid":        map[string]interface{}{"project": "g/p", "discussion_id": "d", "body": "b"},
		"missing discussion": map[string]interface{}{"project": "g/p", "iid": 1, "body": "b"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp, err := handler(context.Background(), wsRequest(t, ws.ActionGitLabMRDiscussionNew, payload))
			wsErr(t, resp, err, ws.ActionGitLabMRDiscussionNew, ws.ErrorCodeBadRequest,
				"project, iid, discussion_id required")
		})
	}
}

// TestWSNewDiscussionNoteRejectsBlankBody covers the separate whitespace guard:
// a body of only spaces is rejected with its own reason and never reaches the
// client, so no empty comment is posted upstream.
func TestWSNewDiscussionNoteRejectsBlankBody(t *testing.T) {
	svc, client, log := newActionFixture(t)

	resp, err := wsNewDiscussionNote(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRDiscussionNew, map[string]interface{}{
			"project": "group/p", "iid": 9, "discussion_id": "disc-1", "body": "   \t\n ",
		}))

	wsErr(t, resp, err, ws.ActionGitLabMRDiscussionNew, ws.ErrorCodeBadRequest, "body required")
	if len(client.noteCalls) != 0 {
		t.Errorf("note calls = %+v, want none for a blank body", client.noteCalls)
	}
}

func TestWSResolveDiscussionForwardsDiscussionID(t *testing.T) {
	svc, client, log := newActionFixture(t)

	resp, err := wsResolveDiscussion(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRDiscussionResolve, map[string]interface{}{
			"project": "group/p", "iid": 9, "discussion_id": "disc-1",
		}))

	var body map[string]bool
	wsOK(t, resp, err, ws.ActionGitLabMRDiscussionResolve, &body)
	if !body["resolved"] {
		t.Errorf("payload = %v, want {\"resolved\":true}", body)
	}
	if len(client.resolveCalls) != 1 ||
		client.resolveCalls[0] != (resolveCall{Project: "group/p", IID: 9, DiscussionID: "disc-1"}) {
		t.Errorf("resolve calls = %+v, want the exact request arguments", client.resolveCalls)
	}
}

func TestWSResolveDiscussionRequiresDiscussionID(t *testing.T) {
	svc, _, log := newActionFixture(t)

	resp, err := wsResolveDiscussion(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRDiscussionResolve, map[string]interface{}{
			"project": "group/p", "iid": 9,
		}))

	wsErr(t, resp, err, ws.ActionGitLabMRDiscussionResolve, ws.ErrorCodeBadRequest,
		"project, iid, discussion_id required")
}

// --- Merge methods ---

func TestWSGetProjectMergeMethodsReturnsAllowedMethods(t *testing.T) {
	svc, client, log := newActionFixture(t)
	client.mergeMethods = &ProjectMergeMethods{Merge: true, AllowSquash: true}

	resp, err := wsGetProjectMergeMethods(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabProjectMergeMethodsGet, map[string]string{"project": "group/p"}))

	var methods ProjectMergeMethods
	wsOK(t, resp, err, ws.ActionGitLabProjectMergeMethodsGet, &methods)
	if !methods.Merge || !methods.AllowSquash || methods.FastForward {
		t.Errorf("methods = %+v, want merge+squash only", methods)
	}
}

func TestWSGetProjectMergeMethodsRequiresProject(t *testing.T) {
	svc, _, log := newActionFixture(t)

	resp, err := wsGetProjectMergeMethods(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabProjectMergeMethodsGet, map[string]string{}))

	wsErr(t, resp, err, ws.ActionGitLabProjectMergeMethodsGet, ws.ErrorCodeBadRequest, "project required")
}

func TestWSGetProjectMergeMethodsSurfacesUpstreamFailure(t *testing.T) {
	svc, client, log := newActionFixture(t)
	client.mergeErr = errors.New("gitlab unreachable")

	resp, err := wsGetProjectMergeMethods(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabProjectMergeMethodsGet, map[string]string{"project": "group/p"}))

	wsErr(t, resp, err, ws.ActionGitLabProjectMergeMethodsGet, ws.ErrorCodeInternalError, "gitlab unreachable")
}

// --- Projects ---

func TestWSListUserProjectsReturnsProjects(t *testing.T) {
	svc, _, log := newActionFixture(t)

	resp, err := wsListUserProjects(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabListUserProjects, nil))

	var body struct {
		Projects []Project `json:"projects"`
	}
	wsOK(t, resp, err, ws.ActionGitLabListUserProjects, &body)
	if len(body.Projects) != 1 || body.Projects[0].PathWithNamespace != "kandev/sample" {
		t.Fatalf("projects = %+v, want the single kandev/sample project", body.Projects)
	}
}

func TestWSSearchProjectsFiltersByQuery(t *testing.T) {
	svc, _, log := newActionFixture(t)
	handler := wsSearchProjects(svc, log)

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"matching query", "sample", 1},
		{"non-matching query", "nothing-like-this", 0},
		{"empty query returns all", "", 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := handler(context.Background(),
				wsRequest(t, ws.ActionGitLabSearchProjects, map[string]interface{}{
					"query": test.query, "limit": 10,
				}))

			var body struct {
				Projects []Project `json:"projects"`
			}
			wsOK(t, resp, err, ws.ActionGitLabSearchProjects, &body)
			if len(body.Projects) != test.want {
				t.Errorf("projects = %d, want %d (%+v)", len(body.Projects), test.want, body.Projects)
			}
		})
	}
}

func TestWSSearchProjectsRejectsMalformedPayload(t *testing.T) {
	svc, _, log := newActionFixture(t)

	resp, err := wsSearchProjects(svc, log)(context.Background(),
		wsRawRequest(ws.ActionGitLabSearchProjects, `{"limit":"ten"}`))

	wsErr(t, resp, err, ws.ActionGitLabSearchProjects, ws.ErrorCodeBadRequest, errMsgInvalidPayload)
}

func TestWSListProjectBranchesReturnsBranches(t *testing.T) {
	svc, client, log := newActionFixture(t)
	client.SeedBranches("group/p", []RepoBranch{{Name: "main"}, {Name: "dev"}})

	resp, err := wsListProjectBranches(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabProjectBranches, map[string]string{"project": "group/p"}))

	var body struct {
		Branches []RepoBranch `json:"branches"`
	}
	wsOK(t, resp, err, ws.ActionGitLabProjectBranches, &body)
	if len(body.Branches) != 2 || body.Branches[0].Name != "main" {
		t.Fatalf("branches = %+v, want [main dev]", body.Branches)
	}
}

func TestWSListProjectBranchesRequiresProject(t *testing.T) {
	svc, _, log := newActionFixture(t)

	resp, err := wsListProjectBranches(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabProjectBranches, map[string]string{}))

	wsErr(t, resp, err, ws.ActionGitLabProjectBranches, ws.ErrorCodeBadRequest, "project required")
}

// --- Action presets ---

func TestWSListActionPresetsReturnsDefaultsForNewWorkspace(t *testing.T) {
	svc, _, _ := newWSFixture(t, NewMockClient(DefaultHost))

	resp, err := wsListActionPresets(svc)(context.Background(),
		wsRequest(t, ws.ActionGitLabActionPresetsList, map[string]string{"workspace_id": "ws-1"}))

	var presets ActionPresets
	wsOK(t, resp, err, ws.ActionGitLabActionPresetsList, &presets)
	if len(presets.MR) == 0 || len(presets.Issue) == 0 {
		t.Fatalf("presets = %+v, want the built-in defaults substituted", presets)
	}
}

func TestWSListActionPresetsRequiresWorkspaceID(t *testing.T) {
	svc, _, _ := newWSFixture(t, NewMockClient(DefaultHost))

	resp, err := wsListActionPresets(svc)(context.Background(),
		wsRequest(t, ws.ActionGitLabActionPresetsList, map[string]string{}))

	wsErr(t, resp, err, ws.ActionGitLabActionPresetsList, ws.ErrorCodeBadRequest, "workspace_id required")
}

func TestWSUpdateActionPresetsPersistsMRList(t *testing.T) {
	svc, store, _ := newWSFixture(t, NewMockClient(DefaultHost))
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")

	resp, err := wsUpdateActionPresets(svc)(ctx,
		wsRequest(t, ws.ActionGitLabActionPresetsUpdate, UpdateActionPresetsRequest{
			WorkspaceID: "ws-1",
			MR:          &[]ActionPreset{{ID: "ship", Label: "Ship it", PromptTemplate: "ship {{mr}}"}},
		}))

	var presets ActionPresets
	wsOK(t, resp, err, ws.ActionGitLabActionPresetsUpdate, &presets)
	if len(presets.MR) != 1 || presets.MR[0].ID != "ship" {
		t.Fatalf("mr presets = %+v, want only the custom ship preset", presets.MR)
	}
	stored, err := store.GetActionPresets(ctx, "ws-1")
	if err != nil {
		t.Fatalf("reload presets: %v", err)
	}
	if len(stored.MR) != 1 || stored.MR[0].Label != "Ship it" {
		t.Errorf("stored mr presets = %+v, want the custom preset persisted", stored.MR)
	}
}

func TestWSUpdateActionPresetsRequiresWorkspaceID(t *testing.T) {
	svc, _, _ := newWSFixture(t, NewMockClient(DefaultHost))

	resp, err := wsUpdateActionPresets(svc)(context.Background(),
		wsRequest(t, ws.ActionGitLabActionPresetsUpdate, UpdateActionPresetsRequest{}))

	wsErr(t, resp, err, ws.ActionGitLabActionPresetsUpdate, ws.ErrorCodeBadRequest, "workspace_id required")
}

func TestWSResetActionPresetsClearsStoredOverrides(t *testing.T) {
	svc, store, _ := newWSFixture(t, NewMockClient(DefaultHost))
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	if err := store.UpsertActionPresets(ctx, &ActionPresets{
		WorkspaceID: "ws-1",
		MR:          []ActionPreset{{ID: "ship", Label: "Ship it", PromptTemplate: "ship"}},
	}); err != nil {
		t.Fatalf("seed presets: %v", err)
	}

	resp, err := wsResetActionPresets(svc)(ctx,
		wsRequest(t, ws.ActionGitLabActionPresetsReset, map[string]string{"workspace_id": "ws-1"}))

	var presets ActionPresets
	wsOK(t, resp, err, ws.ActionGitLabActionPresetsReset, &presets)
	for _, preset := range presets.MR {
		if preset.ID == "ship" {
			t.Fatalf("mr presets = %+v, want the custom preset gone after reset", presets.MR)
		}
	}
	stored, err := store.GetActionPresets(ctx, "ws-1")
	if err != nil {
		t.Fatalf("reload presets: %v", err)
	}
	if len(stored.MR) != 0 {
		t.Errorf("stored mr presets = %+v, want the row deleted", stored.MR)
	}
}

func TestWSResetActionPresetsRequiresWorkspaceID(t *testing.T) {
	svc, _, _ := newWSFixture(t, NewMockClient(DefaultHost))

	resp, err := wsResetActionPresets(svc)(context.Background(),
		wsRequest(t, ws.ActionGitLabActionPresetsReset, map[string]string{}))

	wsErr(t, resp, err, ws.ActionGitLabActionPresetsReset, ws.ErrorCodeBadRequest, "workspace_id required")
}

// --- Cleanup ---

// cleanupTaskDeleter records the task IDs a cleanup sweep deletes.
type cleanupTaskDeleter struct{ ids []string }

func (d *cleanupTaskDeleter) DeleteTask(_ context.Context, taskID string) error {
	d.ids = append(d.ids, taskID)
	return nil
}

func TestWSCleanupReviewTasksDeletesTasksForMergedMRs(t *testing.T) {
	client := NewMockClient(DefaultHost)
	svc, store, _ := newWSFixture(t, client)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	deleter := &cleanupTaskDeleter{}
	svc.SetTaskDeleter(deleter)

	const project, iid = "team/repo", 7
	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyAlways}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	if _, err := store.ReserveReviewMRTask(ctx, watch.ID, project, iid, "url"); err != nil {
		t.Fatalf("reserve review MR task: %v", err)
	}
	if err := store.AssignReviewMRTaskID(ctx, watch.ID, project, iid, "task-merged"); err != nil {
		t.Fatalf("assign review MR task: %v", err)
	}
	client.SeedMR(project, &MR{IID: iid, State: gitlabStateMerged})

	resp, err := wsCleanupReviewTasks(svc)(ctx,
		wsRequest(t, ws.ActionGitLabCleanupReviewTasks, nil))

	var body map[string]int
	wsOK(t, resp, err, ws.ActionGitLabCleanupReviewTasks, &body)
	if body[respKeyDeleted] != 1 {
		t.Errorf("payload = %v, want {%q:1}", body, respKeyDeleted)
	}
	if len(deleter.ids) != 1 || deleter.ids[0] != "task-merged" {
		t.Errorf("deleted task ids = %v, want [task-merged]", deleter.ids)
	}
}

func TestWSCleanupReviewTasksReportsMissingDeleterAsInternalError(t *testing.T) {
	svc, _, _ := newWSFixture(t, NewMockClient(DefaultHost))

	resp, err := wsCleanupReviewTasks(svc)(context.Background(),
		wsRequest(t, ws.ActionGitLabCleanupReviewTasks, nil))

	wsErr(t, resp, err, ws.ActionGitLabCleanupReviewTasks, ws.ErrorCodeInternalError,
		"task deleter not configured")
}

func TestWSCleanupIssueTasksDeletesTasksForClosedIssues(t *testing.T) {
	client := NewMockClient(DefaultHost)
	svc, store, _ := newWSFixture(t, client)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	deleter := &cleanupTaskDeleter{}
	svc.SetTaskDeleter(deleter)

	const project, iid = "team/repo", 5
	watch := &IssueWatch{WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyAlways}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("create issue watch: %v", err)
	}
	if _, err := store.ReserveIssueWatchTask(ctx, watch.ID, project, iid, "url"); err != nil {
		t.Fatalf("reserve issue task: %v", err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, project, iid, "task-closed"); err != nil {
		t.Fatalf("assign issue task: %v", err)
	}
	client.SeedIssue(project, &Issue{IID: iid, State: gitlabStateClosed})

	resp, err := wsCleanupIssueTasks(svc)(ctx,
		wsRequest(t, ws.ActionGitLabCleanupIssueTasks, nil))

	var body map[string]int
	wsOK(t, resp, err, ws.ActionGitLabCleanupIssueTasks, &body)
	if body[respKeyDeleted] != 1 {
		t.Errorf("payload = %v, want {%q:1}", body, respKeyDeleted)
	}
	if len(deleter.ids) != 1 || deleter.ids[0] != "task-closed" {
		t.Errorf("deleted task ids = %v, want [task-closed]", deleter.ids)
	}
}

func TestWSCleanupIssueTasksReportsMissingDeleterAsInternalError(t *testing.T) {
	svc, _, _ := newWSFixture(t, NewMockClient(DefaultHost))

	resp, err := wsCleanupIssueTasks(svc)(context.Background(),
		wsRequest(t, ws.ActionGitLabCleanupIssueTasks, nil))

	wsErr(t, resp, err, ws.ActionGitLabCleanupIssueTasks, ws.ErrorCodeInternalError,
		"task deleter not configured")
}
