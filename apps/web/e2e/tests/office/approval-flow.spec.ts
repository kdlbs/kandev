import { test, expect } from "../../fixtures/office-fixture";

/**
 * E2E coverage for the office task approval gate (in_review → done is
 * blocked until every approver has a current "approved" decision). Spec:
 *   docs/specs/office-task-approval-flow/spec.md
 *
 * Approach:
 *  - Use `officeApi.createAgent` to add an approver to the workspace.
 *  - Use `POST /api/v1/office/tasks/:id/approvers` to attach the approver
 *    to the task. (The office-extended-api uses the /api/v1/office prefix.)
 *  - Drive the task into in_review via the status picker, then click
 *    "Done" and assert the toast + status rollback.
 *  - Decisions are recorded via `POST /api/v1/office/tasks/:id/approve`
 *    with `X-Office-User-Caller` so the backend treats the call as the
 *    singleton human user (matches the v1 unauthenticated frontend path).
 */

test.describe("Office task approval flow", () => {
  test("gated done transition: 409, toast, status rolls back", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const approver = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Approval Gate CEO",
      role: "worker",
    })) as Record<string, unknown>;
    const approverId = approver.id as string;
    expect(approverId).toBeTruthy();

    const task = await apiClient.createTask(officeSeed.workspaceId, "Approval Gated Task", {
      workflow_id: officeSeed.workflowId,
    });

    // Attach the approver to the task.
    const attach = await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/approvers`, {
      agent_profile_id: approverId,
    });
    expect(attach.ok).toBe(true);

    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Approval Gated Task" })).toBeVisible({
      timeout: 10_000,
    });

    // Move the task to in_review via the picker so the gated transition
    // exercises `done`. The gate fires only when leaving in_review for done.
    await testPage.getByTestId("status-picker-trigger").click();
    await testPage.getByTestId("status-picker-option-in_review").click();
    await expect(testPage.getByTestId("status-picker-trigger")).toContainText(/In Review/i, {
      timeout: 5_000,
    });

    // Click "Done" — the backend must reject with 409 + pending_approvers,
    // the optimistic mutation rolls back, and a toast surfaces.
    await testPage.getByTestId("status-picker-trigger").click();
    await testPage.getByTestId("status-picker-option-done").click();

    // The toast text shape is "Cannot mark done: awaiting approval from <names>".
    await expect(testPage.getByText(/awaiting approval from/i).first()).toBeVisible({
      timeout: 10_000,
    });

    // The picker rolls back to In Review (the optimistic-update hook
    // restores the prior snapshot on failure).
    await expect(testPage.getByTestId("status-picker-trigger")).toContainText(/In Review/i, {
      timeout: 5_000,
    });

    // Confirm the backend kept the status at in_review (the redirect from
    // the 409 — the spec calls this out as a convenience).
    const persisted = (await officeApi.getTask(task.id)) as Record<string, unknown>;
    const inner = (persisted.task as Record<string, unknown>) ?? persisted;
    const status = (inner.status as string) ?? "";
    expect(status.toLowerCase()).toContain("review");
  });

  test("approval clears the gate: status moves to done", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const approver = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Approval Clearing CEO",
      role: "worker",
    })) as Record<string, unknown>;
    const approverId = approver.id as string;

    const task = await apiClient.createTask(officeSeed.workspaceId, "Approval Clearing Task", {
      workflow_id: officeSeed.workflowId,
    });
    const attach = await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/approvers`, {
      agent_profile_id: approverId,
    });
    expect(attach.ok).toBe(true);

    // Move to in_review via API (faster than UI).
    const moveToReview = await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      status: "in_review",
    });
    expect(moveToReview.ok).toBe(true);

    // Record an approval as the approver agent. The backend's
    // `resolveDeciderCaller` recognises the agent ID via the agent_caller
    // middleware; absent that, the X-Office-User-Caller header treats the
    // request as the singleton user. We use the agent header form here so
    // the decision is recorded as the approver.
    const approveRes = await fetch(
      `${(apiClient as unknown as { baseUrl: string }).baseUrl}/api/v1/office/tasks/${task.id}/approve`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Office-User-Caller": approverId,
        },
        body: JSON.stringify({ comment: "looks good" }),
      },
    );
    expect(approveRes.ok).toBe(true);

    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Approval Clearing Task" })).toBeVisible({
      timeout: 10_000,
    });

    // Click status picker → Done. With every approver having an "approved"
    // decision, the backend allows the transition.
    await testPage.getByTestId("status-picker-trigger").click();
    await testPage.getByTestId("status-picker-option-done").click();

    await expect(testPage.getByTestId("status-picker-trigger")).toContainText(/Done/i, {
      timeout: 10_000,
    });

    // Confirm via API.
    const persisted = (await officeApi.getTask(task.id)) as Record<string, unknown>;
    const inner = (persisted.task as Record<string, unknown>) ?? persisted;
    const status = (inner.status as string) ?? "";
    expect(status.toLowerCase()).toBe("done");
  });

  test("request-changes records a decision visible in the timeline and queues a run for the assignee", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Create a fresh assignee agent for this test. The worker-scoped
    // seeded CEO is wiped by `cleanupTestProfiles` between tests, so
    // referencing officeSeed.agentId here yields an agent the runtime
    // can no longer find when the reactivity pipeline tries to wake it.
    const assignee = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Changes Assignee",
      role: "worker",
    })) as Record<string, unknown>;
    const assigneeId = assignee.id as string;
    expect(assigneeId).toBeTruthy();

    // Add a separate reviewer agent.
    const reviewer = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Changes Reviewer",
      role: "worker",
    })) as Record<string, unknown>;
    const reviewerId = reviewer.id as string;

    const task = await apiClient.createTask(officeSeed.workspaceId, "Request Changes Task", {
      workflow_id: officeSeed.workflowId,
    });

    // Assign the task to the CEO + attach the reviewer.
    const assign = await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      assignee_agent_profile_id: assigneeId,
    });
    expect(assign.ok).toBe(true);
    const attach = await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/reviewers`, {
      agent_profile_id: reviewerId,
    });
    expect(attach.ok).toBe(true);

    // The reactivity pipeline only wakes reviewers/approvers when the
    // task transitions into in_review. Drive it there before requesting
    // changes so the run queue assertions are deterministic.
    const review = await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      status: "in_review",
    });
    expect(review.ok).toBe(true);

    // Record a request-changes decision as the reviewer. Comment is
    // required for request-changes per the spec.
    const baseUrl = (apiClient as unknown as { baseUrl: string }).baseUrl;
    const decisionRes = await fetch(`${baseUrl}/api/v1/office/tasks/${task.id}/request-changes`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Office-User-Caller": reviewerId,
      },
      body: JSON.stringify({ comment: "please update the docs" }),
    });
    expect(decisionRes.ok).toBe(true);

    // 1. UI surface: the decision renders in the comments timeline.
    //    `formatDecisionLine` produces "<deciderName> requested changes: '<comment>'".
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Request Changes Task" })).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByText(/requested changes/i).first()).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText(/please update the docs/i).first()).toBeVisible({
      timeout: 10_000,
    });

    // 2. Backend surface: a `task_changes_requested` run landed in the
    //    queue addressed to the assignee. Polled via the existing public
    //    /workspaces/:wsId/runs endpoint to absorb the small WS / bus
    //    delay between the POST and the run queue insert.
    await expect
      .poll(
        async () => {
          const res = await fetch(
            `${baseUrl}/api/v1/office/workspaces/${officeSeed.workspaceId}/runs`,
          );
          if (!res.ok) return false;
          const body = (await res.json()) as {
            runs?: Array<{
              agent_profile_id?: string;
              task_id?: string;
              reason?: string;
            }>;
          };
          return (body.runs ?? []).some(
            (w) =>
              w.agent_profile_id === assigneeId &&
              w.task_id === task.id &&
              w.reason === "task_changes_requested",
          );
        },
        { timeout: 10_000 },
      )
      .toBe(true);
  });
});
