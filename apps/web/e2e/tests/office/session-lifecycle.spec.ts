import { test, expect } from "../../fixtures/office-fixture";

/**
 * E2E coverage for the office task session lifecycle (per-(task, agent)
 * persistent sessions, RUNNING ↔ IDLE state cycling, COMPLETED only on
 * participation removal). Spec:
 *   docs/specs/office-task-session-lifecycle/spec.md
 *
 * Harness limitations:
 *  - The /api/v1/_test/task-sessions route only INSERTS new rows; it does
 *    not update an existing row's state. Tests work around this by:
 *      a) seeding a session with the desired state directly, OR
 *      b) using the real reactivity endpoints (assignee change, reviewer
 *         removal) and asserting the row's final state via the public
 *         `/api/v1/tasks/:id/sessions` endpoint.
 *  - IDLE-from-RUNNING transition for a single seeded row is not directly
 *    expressible — the harness has no UPDATE path. The fire-and-forget
 *    test asserts spinner visibility on a freshly-seeded RUNNING row, and
 *    then verifies a freshly-seeded IDLE row does NOT light the spinner.
 *    Both observations validate the same selector contract
 *    (`selectLiveSessionForTask` only treats RUNNING as live for office).
 */

test.describe("Office task session lifecycle", () => {
  test("fire-and-forget: RUNNING lights the spinner, IDLE does not", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(
      officeSeed.workspaceId,
      "Lifecycle Fire-and-Forget Task",
      { workflow_id: officeSeed.workflowId },
    );

    // RUNNING: topbar spinner visible, timeline entry expanded.
    const startedAt = new Date(Date.now() - 30_000).toISOString();
    const { session_id } = await apiClient.seedTaskSession(task.id, {
      state: "RUNNING",
      agentProfileId: officeSeed.agentId,
      startedAt,
    });

    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(
      testPage.getByRole("heading", { name: "Lifecycle Fire-and-Forget Task" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(testPage.getByTestId("topbar-working-active")).toBeVisible({
      timeout: 10_000,
    });
    // Office sessions surface as per-agent tabs in chat-activity-tabs
    // since the Chat tab refactor — the tab id is the representative
    // session id (for a single session that matches session_id) and
    // its content is an AdvancedChatPanel embed keyed by the active
    // session id.
    await testPage.getByTestId(`agent-tab-${session_id}`).click();
    await expect(testPage.getByTestId(`agent-tab-embed-${session_id}`)).toBeVisible({
      timeout: 10_000,
    });

    // Now seed a SECOND task with an IDLE session and assert the spinner
    // is absent. (We can't update the original row in place — see file
    // header for harness limitation.)
    const idleTask = await apiClient.createTask(officeSeed.workspaceId, "Lifecycle Idle Task", {
      workflow_id: officeSeed.workflowId,
    });
    const { session_id: idleSessionId } = await apiClient.seedTaskSession(idleTask.id, {
      state: "IDLE",
      agentProfileId: officeSeed.agentId,
      startedAt,
    });

    await testPage.goto(`/office/tasks/${idleTask.id}`);
    await expect(testPage.getByRole("heading", { name: "Lifecycle Idle Task" })).toBeVisible({
      timeout: 10_000,
    });

    // IDLE office session does NOT count as live → no topbar spinner.
    await expect(testPage.getByTestId("topbar-working-indicator")).toHaveCount(0);

    // IDLE office sessions still produce a per-agent tab — IDLE is a
    // paused conversation, not a deletion. Click the tab and confirm
    // its embed mounts.
    await testPage.getByTestId(`agent-tab-${idleSessionId}`).click();
    await expect(testPage.getByTestId(`agent-tab-embed-${idleSessionId}`)).toBeVisible({
      timeout: 10_000,
    });
  });

  test("multi-agent: assignee + reviewer get separate timeline entries", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Add a second agent to act as reviewer.
    const reviewerAgent = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Lifecycle Reviewer",
      role: "worker",
    })) as Record<string, unknown>;
    const reviewerId = reviewerAgent.id as string;
    expect(reviewerId).toBeTruthy();

    const task = await apiClient.createTask(officeSeed.workspaceId, "Lifecycle Multi Agent Task", {
      workflow_id: officeSeed.workflowId,
    });

    // Add the reviewer to the task's reviewers list.
    const addRes = await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/reviewers`, {
      agent_profile_id: reviewerId,
    });
    expect(addRes.ok).toBe(true);

    // Seed two RUNNING sessions on the same task — one per agent.
    const startedAt = new Date(Date.now() - 60_000).toISOString();
    const assigneeSeed = await apiClient.seedTaskSession(task.id, {
      state: "RUNNING",
      agentProfileId: officeSeed.agentId,
      startedAt,
    });
    const reviewerSeed = await apiClient.seedTaskSession(task.id, {
      state: "RUNNING",
      agentProfileId: reviewerId,
      startedAt: new Date(Date.now() - 30_000).toISOString(),
    });

    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Lifecycle Multi Agent Task" })).toBeVisible(
      { timeout: 10_000 },
    );

    // Both per-agent tabs must render (chat-activity-tabs.tsx groups
    // by agent_profile_id, with one tab per group). The tab content
    // is the AdvancedChatPanel embed; clicking each tab swaps the
    // active session-id on the embed without spawning a separate
    // SessionTimelineEntry (that surface was retired from the chat
    // tab — see the chat-activity-tabs commentary).
    const assigneeTab = testPage.getByTestId(`agent-tab-${assigneeSeed.session_id}`);
    const reviewerTab = testPage.getByTestId(`agent-tab-${reviewerSeed.session_id}`);
    await expect(assigneeTab).toBeVisible({ timeout: 10_000 });
    await expect(reviewerTab).toBeVisible({ timeout: 10_000 });

    await assigneeTab.click();
    await expect(testPage.getByTestId(`agent-tab-embed-${assigneeSeed.session_id}`)).toBeVisible({
      timeout: 10_000,
    });

    await reviewerTab.click();
    await expect(testPage.getByTestId(`agent-tab-embed-${reviewerSeed.session_id}`)).toBeVisible({
      timeout: 10_000,
    });

    // Topbar spinner is present (any one RUNNING session is enough).
    await expect(testPage.getByTestId("topbar-working-active")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("reassignment: prev assignee's session moves to a terminal state", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const newAssignee = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Lifecycle Reassignee",
      role: "worker",
    })) as Record<string, unknown>;
    const newAssigneeId = newAssignee.id as string;
    expect(newAssigneeId).toBeTruthy();

    const task = await apiClient.createTask(officeSeed.workspaceId, "Lifecycle Reassignment Task", {
      workflow_id: officeSeed.workflowId,
    });

    // Make CEO the formal assignee so the upcoming PATCH counts as a
    // *reassignment* (prevAssignee non-empty) rather than a first-time
    // assign — the reactivity pipeline only fires session-termination
    // when there is a prior assignee to displace.
    const assignRes = await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      assignee_agent_profile_id: officeSeed.agentId,
    });
    expect(assignRes.ok).toBe(true);

    // Seed a RUNNING session for the original assignee (CEO from seed).
    const startedAt = new Date(Date.now() - 30_000).toISOString();
    const { session_id: originalSessionId } = await apiClient.seedTaskSession(task.id, {
      state: "RUNNING",
      agentProfileId: officeSeed.agentId,
      startedAt,
    });

    // Reassign the task to the new agent via the office task PATCH.
    const patchRes = await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      assignee_agent_profile_id: newAssigneeId,
    });
    expect(patchRes.ok).toBe(true);

    // Verify (via the public listTaskSessions endpoint) that the original
    // session is no longer in a non-terminal state. Per the spec, the
    // reactivity pipeline + participants update terminate the prior
    // assignee's session.
    await expect
      .poll(
        async () => {
          const list = await apiClient.listTaskSessions(task.id);
          const original = list.sessions.find((s) => s.id === originalSessionId);
          return original?.state ?? "missing";
        },
        { timeout: 10_000, intervals: [500, 1000] },
      )
      .toMatch(/^(COMPLETED|CANCELLED|FAILED)$/);
  });

  test("reviewer removal terminates the reviewer's session", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const reviewerAgent = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Lifecycle Reviewer For Removal",
      role: "worker",
    })) as Record<string, unknown>;
    const reviewerId = reviewerAgent.id as string;
    expect(reviewerId).toBeTruthy();

    const task = await apiClient.createTask(
      officeSeed.workspaceId,
      "Lifecycle Reviewer Removal Task",
      { workflow_id: officeSeed.workflowId },
    );

    const addRes = await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/reviewers`, {
      agent_profile_id: reviewerId,
    });
    expect(addRes.ok).toBe(true);

    // Seed a RUNNING session for the reviewer.
    const startedAt = new Date(Date.now() - 30_000).toISOString();
    const { session_id: reviewerSessionId } = await apiClient.seedTaskSession(task.id, {
      state: "RUNNING",
      agentProfileId: reviewerId,
      startedAt,
    });

    // Remove the reviewer via DELETE /tasks/:id/reviewers/:agentId.
    const removeRes = await apiClient.rawRequest(
      "DELETE",
      `/api/v1/office/tasks/${task.id}/reviewers/${reviewerId}`,
    );
    expect(removeRes.ok).toBe(true);

    // The reviewer's session must transition to a terminal state.
    await expect
      .poll(
        async () => {
          const list = await apiClient.listTaskSessions(task.id);
          const reviewerSession = list.sessions.find((s) => s.id === reviewerSessionId);
          return reviewerSession?.state ?? "missing";
        },
        { timeout: 10_000, intervals: [500, 1000] },
      )
      .toMatch(/^(COMPLETED|CANCELLED|FAILED)$/);
  });
});
