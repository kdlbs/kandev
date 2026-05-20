import { test, expect } from "../../fixtures/office-fixture";

/**
 * Regression net for fixes shipped after the office lifecycle work
 * landed. Each test pins one user-visible bug we already shipped a fix
 * for, so a refactor that re-introduces the bug fails CI before it
 * reaches production.
 *
 *   - Agent runs tab snake_case mismatch (silent "No runs yet")
 *   - Comment input hidden on office tasks
 *   - Topbar disappears on /office/tasks/:id
 *   - Role-aware avatar shown on agent detail page
 */

test.describe("Office regression net", () => {
  test("agent /runs tab shows runs when the agent has actually run", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    // Seed: create a task assigned to the office seed CEO. Then seed
    // a finished session so a run row exists in the DB. The runs
    // tab queries /api/v1/office/workspaces/:wsId/runs and filters
    // by agent_profile_id. Pre-fix this filter looked at camelCase
    // `agentProfileId` which the backend never returns — every row
    // was filtered out.
    const task = await apiClient.createTask(officeSeed.workspaceId, "Runs tab regression task", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      assignee_agent_profile_id: officeSeed.agentId,
    });
    await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      agentProfileId: officeSeed.agentId,
      startedAt: new Date(Date.now() - 30_000).toISOString(),
      completedAt: new Date(Date.now() - 5_000).toISOString(),
    });
    // The run queue is what the runs tab actually reads; create one
    // by issuing a comment which fires the reactivity-pipeline run.
    await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/comments`, {
      body: "trigger a task_comment run",
    });

    await testPage.goto(`/office/agents/${officeSeed.agentId}`);

    // Click the Runs sub-route nav link — the page lands on the
    // dashboard by default. The agent detail nav was tabs in the
    // pre-refactor UI; it is now a link list aria-labelled
    // "Agent sections".
    await testPage
      .getByRole("navigation", { name: "Agent sections" })
      .getByRole("link", { name: /runs/i })
      .click();

    // The run should be listed by reason. The runs UI pretty-prints
    // reasons (`task_comment` → "Task comment"), so the regex covers
    // both the raw snake_case and the formatted form. "No runs yet"
    // must NOT be visible — this is the pre-fix symptom.
    await expect(
      testPage.getByText(/Task comment|Task assigned|task_comment|task_assigned/i).first(),
    ).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText(/no runs yet/i)).toHaveCount(0);
  });

  test("comment input is visible on a completed office task", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Office tasks must always accept comments — a comment is the
    // canonical way to wake the agent again. The pre-fix bug:
    // useChatReadOnly hid the input when the most recent session was
    // terminal (COMPLETED/FAILED/CANCELLED), so once the agent finished
    // a turn the user could no longer reply.
    const task = await apiClient.createTask(
      officeSeed.workspaceId,
      "Comment input regression task",
      { workflow_id: officeSeed.workflowId },
    );
    await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      assignee_agent_profile_id: officeSeed.agentId,
      project_id: officeSeed.projectId,
    });
    // Seed a COMPLETED session so the "last session is terminal" path
    // is exercised.
    await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      agentProfileId: officeSeed.agentId,
      startedAt: new Date(Date.now() - 30_000).toISOString(),
      completedAt: new Date(Date.now() - 5_000).toISOString(),
    });

    await testPage.goto(`/office/tasks/${task.id}`);

    // The comment input must be present. Placeholder text "Add a
    // comment..." comes from task-chat.tsx ChatInput.
    await expect(testPage.getByPlaceholder(/add a comment/i)).toBeVisible({ timeout: 10_000 });

    // Sanity: also check we landed on the right task.
    await expect(
      testPage.getByRole("heading", { name: /Comment input regression task/i }),
    ).toBeVisible();

    // Ensure the office API still reports the task too.
    const fetched = (await officeApi.getTask(task.id)) as Record<string, unknown>;
    expect(fetched).toBeTruthy();
  });

  test("task detail page renders the topbar breadcrumb", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    // The topbar disappeared once because office-topbar.tsx's
    // isDetailPage regex still matched /office/issues/ after the
    // issues→tasks rename. Pin the breadcrumb visibility on the
    // new route shape — without it, the topbar is gone.
    const task = await apiClient.createTask(officeSeed.workspaceId, "Topbar visible task", {
      workflow_id: officeSeed.workflowId,
    });

    await testPage.goto(`/office/tasks/${task.id}`);

    // Breadcrumb is rendered into the office topbar slot — it has a
    // <nav aria-label="breadcrumb"> wrapper that disappears when the
    // topbar bug regresses.
    await expect(testPage.getByRole("navigation", { name: /breadcrumb/i })).toBeVisible({
      timeout: 10_000,
    });
    // Inside the breadcrumb the parent "Tasks" link must be present.
    await expect(
      testPage.getByRole("navigation", { name: /breadcrumb/i }).getByRole("link", {
        name: /^Tasks$/,
      }),
    ).toBeVisible();
  });

  test("agent detail page renders an initials avatar (no generic robot icon)", async ({
    apiClient,
    officeApi,
    testPage,
    officeSeed,
  }) => {
    // Pre-fix every agent got the same generic IconRobot. The new
    // <AgentAvatar> renders 1–2 initials in a per-name tinted square.
    // Seed a fresh agent with a known name so the test is robust to
    // prior renames in the same worker (the seeded CEO can be
    // "CEO Updated" / "Updated Name" depending on test ordering).
    void apiClient;
    const agent = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Initials Probe",
      role: "worker",
    })) as { id: string };

    await testPage.goto(`/office/agents/${agent.id}`);

    // "Initials Probe" → first letter of each word → "IP".
    await expect(testPage.getByText(/^IP$/, { exact: true }).first()).toBeVisible({
      timeout: 10_000,
    });
    // The old generic robot icon should not appear anywhere on the
    // agent detail page.
    await expect(testPage.locator('svg[class*="tabler-icon-robot"]')).toHaveCount(0);
  });
});
