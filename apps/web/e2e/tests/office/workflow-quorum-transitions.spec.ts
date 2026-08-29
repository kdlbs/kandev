import { test, expect } from "../../fixtures/office-fixture";
import { dwell } from "../../helpers/causal-waits";
import type { ApiClient } from "../../helpers/api-client";

/**
 * E2E coverage for the workflow engine's guarded `move_to_step` transitions
 * (the office-default template's Review/Approval steps, each gated by a
 * `wait_for_quorum` action). Spec:
 *   docs/specs/tasks/requirements/workflow-quorum-decision-recording.md
 *
 * Drives Work -> Review -> Approval with a reviewer and an approver
 * attached, asserting advance-on-approve, return-on-reject, and the AC-25
 * awaiting-decisions presentation (including the AC-61 role switch once the
 * Review guard clears and the Approval guard becomes current). The same
 * spec covers AC-56 at no extra cost: after the return-on-reject leg,
 * re-entering the guarded step must not immediately bounce again.
 *
 * Entry into Review/Approval is driven by `POST /tasks/:id/move` (the same
 * endpoint the kanban board uses for a manual drag) rather than the office
 * `PATCH .../status` endpoint: office-default's Review/Approval steps set
 * `allow_manual_move: true` specifically to permit this, and — unlike the
 * status PATCH, which only updates the legacy `task.status` column and logs
 * activity — a step move fires the engine's real `on_enter` events (e.g.
 * Review's `clear_decisions`), which AC-56 depends on.
 *
 * `office-default`'s Work step has no guarded transition (Work -> Review is
 * unconditional), so `GET .../quorum` returning `guards: []` is the signal
 * that a task left Review; a non-empty `guards` list is the signal that it
 * is currently sitting at a guarded step. This is a backend read rather
 * than a UI wait per the repo's causal-waits convention: "the backend
 * reached state X" needs no DOM primitive.
 */

// office-default's step ids are regenerated as fresh UUIDs per workspace
// (createWorkflowFromTemplate), so a step must be resolved by its stable
// `stage_type` hint rather than the YAML template's literal `id: review`.
async function findStepId(
  apiClient: ApiClient,
  workflowId: string,
  stageType: "review" | "approval",
): Promise<string> {
  const { steps } = await apiClient.listWorkflowSteps(workflowId);
  const step = steps.find((s) => s.stage_type === stageType);
  if (!step) throw new Error(`no ${stageType} step found in workflow ${workflowId}`);
  return step.id;
}

// Mirrors the backend's IsTerminalStepName board convention
// (workflow/models/terminal.go) — office-default's step chain has no
// stage_type for its final column, only a recognized terminal name.
const TERMINAL_STEP_NAMES = new Set(["done", "complete", "completed", "approved"]);

async function findDoneStepId(apiClient: ApiClient, workflowId: string): Promise<string> {
  const { steps } = await apiClient.listWorkflowSteps(workflowId);
  const step = steps.find((s) => TERMINAL_STEP_NAMES.has(s.name.trim().toLowerCase()));
  if (!step) throw new Error(`no terminal step found in workflow ${workflowId}`);
  return step.id;
}

// Decisions must be attributed to the reviewer/approver's own role
// (role=reviewer vs role=approver), not the singleton-user shortcut: the
// unauthenticated `X-Office-User-Caller` header always records a decision
// as an implicit approver (resolveDeciderRole in decisions.go), which
// would never satisfy a role=reviewer guard. An agent-scoped decision
// instead requires the same Bearer-JWT identity a real office agent
// authenticates with (AgentAuthMiddleware); the E2E test harness's
// runtime-token mint endpoint (already used by the runtime-*.spec.ts
// suite) is the supported way to obtain one outside a real agent process.
type RecordDecisionOptions = {
  workspaceId: string;
  taskId: string;
  endpoint: "approve" | "request-changes";
  agentProfileId: string;
  comment?: string;
};

async function recordDecision(apiClient: ApiClient, options: RecordDecisionOptions) {
  const { workspaceId, taskId, endpoint, agentProfileId, comment } = options;
  const run = await apiClient.seedRun({
    agentProfileId,
    status: "claimed",
    reason: "quorum_decision",
    taskId,
  });
  const { token } = await apiClient.mintRuntimeToken({
    agentProfileId,
    workspaceId,
    runId: run.run_id,
    taskId,
  });
  const baseUrl = (apiClient as unknown as { baseUrl: string }).baseUrl;
  const res = await fetch(`${baseUrl}/api/v1/office/tasks/${taskId}/${endpoint}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify({ comment }),
  });
  expect(res.ok).toBe(true);

  // The seeded run exists only to mint a decider-scoped JWT; left "claimed"
  // forever, it eventually reads to the office scheduler as an abandoned
  // run and its reactivity pipeline reacts by flipping the legacy status
  // column to "blocked" (apps/backend/internal/office/scheduler/
  // reactivity.go) asynchronously, racing the assertions below. Finishing
  // it here removes that race instead of chasing its timing.
  await apiClient.updateRunStatus(run.run_id, { status: "finished" });
}

async function getQuorumGuards(
  apiClient: { rawRequest: (method: string, path: string) => Promise<Response> },
  workspaceId: string,
  taskId: string,
): Promise<Array<{ role: string; satisfied: boolean; reason?: string }>> {
  const res = await apiClient.rawRequest(
    "GET",
    `/api/v1/office/workspaces/${workspaceId}/tasks/${taskId}/quorum`,
  );
  expect(res.ok).toBe(true);
  const body = (await res.json()) as {
    guards: Array<{ role: string; satisfied: boolean; reason?: string }>;
  };
  return body.guards;
}

test.describe("Office workflow quorum-guarded transitions", () => {
  test("advance-on-approve moves Review to Approval, then Approval to Done", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
    seedData,
  }) => {
    const reviewer = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Quorum Reviewer",
      role: "worker",
    })) as Record<string, unknown>;
    const reviewerId = reviewer.id as string;
    const approver = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Quorum Approver",
      role: "worker",
    })) as Record<string, unknown>;
    const approverId = approver.id as string;

    // EnsureSession requires a resolvable agent profile (PrepareSession
    // rejects an empty one); a task with no assignee and a workspace with no
    // configured default has nothing to resolve otherwise, so the task's own
    // agent_profile_id metadata is what EnsureSession picks up
    // (resolveTaskAgentProfile checks task.Metadata first).
    const task = await apiClient.createTask(officeSeed.workspaceId, "Quorum Advance Task", {
      workflow_id: officeSeed.workflowId,
      agent_profile_id: seedData.agentProfileId,
    });

    // Move to Review BEFORE ensuring a session: only the Work step has the
    // auto_start_agent on-enter action, so EnsureSession would otherwise
    // resolve intent=start and launch through startTask, which requires
    // office-scheduler-injected KANDEV_* runtime env this external HTTP call
    // has no way to supply. Wait for the persisted move before EnsureSession;
    // the move also publishes an asynchronous workflow event. On Review,
    // EnsureSession resolves intent=prepare instead, which only needs a DB row.
    const reviewStepId = await findStepId(apiClient, officeSeed.workflowId, "review");
    await apiClient.moveTask(task.id, officeSeed.workflowId, reviewStepId);
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id)
      .toBe(reviewStepId);

    // AddTaskParticipant binds the new row to the task's CURRENT
    // workflow_step_id (workflow_step_participants.step_id), not a
    // caller-chosen step: register the reviewer only after the move so the
    // row lands under Review's step id — registering beforehand (while the
    // task still sits on Work) would silently orphan it, since
    // resolveDeciderRole's participant lookup filters by the task's
    // present step.
    await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/reviewers`, {
      agent_profile_id: reviewerId,
    });

    // The quorum evaluator resolves a session-scoped machine state (AC-16/
    // F38), so a task with no session at all always yields an empty
    // snapshot regardless of workflow_step_id.
    await apiClient.ensureTaskSession(task.id);

    // AC-25: the Review step's guard is unsatisfied (reviewer has not
    // decided yet), so the diagnostic read reports one awaiting entry.
    await expect
      .poll(
        async () => (await getQuorumGuards(apiClient, officeSeed.workspaceId, task.id))[0]?.role,
      )
      .toBe("reviewer");

    // AC-25 UI presentation: the badge renders the awaiting state.
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Quorum Advance Task" })).toBeVisible({
      timeout: 10_000,
    });
    const badge = testPage.getByTestId("quorum-status-badge");
    await expect(badge).toBeVisible({ timeout: 10_000 });
    await expect(badge).toContainText("Reviewer");
    await expect(badge).toHaveAttribute("data-variant", "outline");

    // The sole reviewer approves: the Review->Approval guard
    // (role=reviewer, threshold=all_approve) is now satisfied and fires.
    await recordDecision(apiClient, {
      workspaceId: officeSeed.workspaceId,
      taskId: task.id,
      endpoint: "approve",
      agentProfileId: reviewerId,
      comment: "looks good",
    });

    // AC-61: the card-level state now reflects the Approval step's guard
    // (role=approver) rather than the cleared Review guard.
    await expect
      .poll(
        async () => (await getQuorumGuards(apiClient, officeSeed.workspaceId, task.id))[0]?.role,
      )
      .toBe("approver");

    // The task now sits on Approval (confirmed by the guard poll above), so
    // registering the approver here binds their participant row to
    // Approval's step id, per the same current-step-scoping rule as the
    // reviewer registration. This must happen before the reload/badge
    // assertion below: with no approver seat registered yet, the AC-50
    // required slate for role=approver is empty, which the engine reports
    // as ReasonSlateEmpty (a genuine "stuck" diagnostic, not
    // threshold_not_met) — registering first is what makes the badge's
    // awaiting-approver state observable at all.
    await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/approvers`, {
      agent_profile_id: approverId,
    });

    await testPage.reload();
    await expect(badge).toContainText("Approver", { timeout: 10_000 });

    // The sole approver approves: Approval->Done fires and the task has no
    // guarded transition left, so the diagnostic read goes empty.
    await recordDecision(apiClient, {
      workspaceId: officeSeed.workspaceId,
      taskId: task.id,
      endpoint: "approve",
      agentProfileId: approverId,
      comment: "shipping it",
    });

    await expect
      .poll(async () => (await getQuorumGuards(apiClient, officeSeed.workspaceId, task.id)).length)
      .toBe(0);

    // Engine-driven transitions (both the ordinary ApplyTransition path and
    // this feature's AC-46/48 compare-and-swap) move workflow_step_id only —
    // syncing the legacy task.status/state column is exclusive to the
    // manual-move service path (MoveTaskWithOptions) and is unrelated,
    // pre-existing behavior this spec does not change. Assert landing via
    // the engine's own record of position instead of the legacy column.
    const doneStepId = await findDoneStepId(apiClient, officeSeed.workflowId);
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id)
      .toBe(doneStepId);

    await testPage.reload();
    await expect(testPage.getByTestId("quorum-status-badge")).toHaveCount(0);
  });

  test("return-on-reject sends the task back to Work, and re-entering Review does not immediately re-bounce (AC-56)", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
    seedData,
  }) => {
    const reviewer = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Quorum Rejecting Reviewer",
      role: "worker",
    })) as Record<string, unknown>;
    const reviewerId = reviewer.id as string;

    const task = await apiClient.createTask(officeSeed.workspaceId, "Quorum Reject Task", {
      workflow_id: officeSeed.workflowId,
      agent_profile_id: seedData.agentProfileId,
    });

    // Move to Review BEFORE ensuring a session: only the Work step has the
    // auto_start_agent on-enter action, so EnsureSession would otherwise
    // resolve intent=start and launch through startTask, which requires
    // office-scheduler-injected KANDEV_* runtime env this external HTTP call
    // has no way to supply. Wait for the persisted move before registering
    // the reviewer; participant rows are scoped to the current step.
    const reviewStepId = await findStepId(apiClient, officeSeed.workflowId, "review");
    await apiClient.moveTask(task.id, officeSeed.workflowId, reviewStepId);
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id)
      .toBe(reviewStepId);

    // AddTaskParticipant binds to the task's CURRENT step id, so the
    // reviewer must be registered after the move lands the task on Review
    // (see the matching comment in the advance-on-approve test above).
    await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/reviewers`, {
      agent_profile_id: reviewerId,
    });

    // The quorum evaluator resolves a session-scoped machine state (AC-16/
    // F38), so a task with no session at all always yields an empty
    // snapshot regardless of workflow_step_id. Seed the row directly rather
    // than calling EnsureSession: a real (or mock-agent) launch finishes its
    // turn and leaves the DB-tracked state within this test's runtime,
    // dropping out of AC-16's active-session set (CREATED/STARTING/RUNNING/
    // WAITING_FOR_INPUT). AC-62's `reevaluation_blocked` is computed live
    // from that same query, so a session that goes idle between setup and
    // the assertions below makes the card render "stuck" — a real, correctly
    // computed state per spec, just not the one this test means to hold
    // constant. A directly seeded row has no lifecycle to decay.
    await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });

    // Nothing past this point needs the task's own agent_profile_id
    // metadata. Clear it: the request-changes decision below also queues a
    // "task_changes_requested" run for whichever agent IS the assignee
    // (decisions.go:runReactivityForDecision), unrelated to the quorum
    // guard this test exercises, so a real agent could act on the
    // feedback. This synthetic HTTP-driven harness has no such agent behind
    // that run, so it fails and its office-scheduler reactivity pipeline
    // flips the legacy status column to "blocked" (reactivity.go's
    // normalisedStatus FAILED->blocked mapping), racing the status-dependent
    // assertions below. With no assignee, decisions.go's own guard
    // (`exec.AssigneeAgentProfileID == ""`) means the run is never queued in
    // the first place.
    const clearAssignee = await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      assignee_agent_profile_id: "",
    });
    expect(clearAssignee.ok).toBe(true);

    await expect
      .poll(async () => (await getQuorumGuards(apiClient, officeSeed.workspaceId, task.id)).length)
      .toBeGreaterThan(0);

    // any_reject fires on a single veto: the task returns to Work, which
    // has no guarded transition, so the diagnostic read goes empty.
    await recordDecision(apiClient, {
      workspaceId: officeSeed.workspaceId,
      taskId: task.id,
      endpoint: "request-changes",
      agentProfileId: reviewerId,
      comment: "please rework this",
    });
    await expect
      .poll(async () => (await getQuorumGuards(apiClient, officeSeed.workspaceId, task.id)).length)
      .toBe(0);

    // Re-enter Review. Its on_enter action clears superseded decisions, so
    // the stale any_reject vote must not immediately fire the same guard
    // again (AC-56) — the fresh read should show an unsatisfied-but-pending
    // reviewer guard, not an empty (already-bounced) one.
    await apiClient.moveTask(task.id, officeSeed.workflowId, reviewStepId);

    await expect
      .poll(
        async () => (await getQuorumGuards(apiClient, officeSeed.workspaceId, task.id))[0]?.reason,
      )
      .toBe("threshold_not_met");

    // Confirm it stays put rather than bouncing back to Work shortly after.
    await dwell(500, "negative-assertion", "asserting the guard does not re-fire on stale state");
    const guardsAfterDwell = await getQuorumGuards(apiClient, officeSeed.workspaceId, task.id);
    expect(guardsAfterDwell[0]?.role).toBe("reviewer");
    expect(guardsAfterDwell[0]?.satisfied).toBe(false);

    // AC-25 UI presentation of the stable awaiting state. The badge fetches
    // by task and workspace, so it does not depend on the legacy status
    // column matching the workflow step.
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Quorum Reject Task" })).toBeVisible({
      timeout: 10_000,
    });
    const badge = testPage.getByTestId("quorum-status-badge");
    await expect(badge).toBeVisible({ timeout: 10_000 });
    await expect(badge).toHaveAttribute("data-variant", "outline");
  });
});
