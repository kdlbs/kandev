import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Tests the session tabs on the kanban right-side preview panel:
 * - Every session of the task shows up as a tab
 * - Clicking a tab switches the rendered session body and updates the URL
 *
 * Session creation and deletion are deliberately NOT exposed in the preview
 * panel — those live on the full-page task view.
 */
test.describe("Preview session tabs", () => {
  test("shows all sessions as tabs and switches between them", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    // 1. Create a task and seed two completed sessions directly. This spec
    // tests read-only preview tabs, not full-page new-session creation.
    const task = await apiClient.createTask(seedData.workspaceId, "Preview Tabs Task", {
      description: "Preview session tabs",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const now = Date.now();
    const primaryId = randomUUID();
    await apiClient.seedTaskSession(task.id, {
      sessionId: primaryId,
      state: "COMPLETED",
      startedAt: new Date(now).toISOString(),
      completedAt: new Date(now + 1_000).toISOString(),
    });
    await apiClient.seedSessionMessage(primaryId, {
      type: "message",
      content: "simple mock response from primary session",
    });
    await apiClient.setPrimarySession(primaryId);

    const secondary = await apiClient.seedTaskSession(task.id, {
      sessionId: randomUUID(),
      state: "COMPLETED",
      startedAt: new Date(now + 2_000).toISOString(),
      completedAt: new Date(now + 3_000).toISOString(),
    });
    const secondaryId = secondary.session_id;
    await apiClient.seedSessionMessage(secondaryId, {
      type: "message",
      content: "secondary-session-response",
    });

    // 2. Wait for both sessions to be visible to the API.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.filter((s) => DONE_STATES.includes(s.state)).length;
        },
        { timeout: 60_000, message: "Waiting for second session to finish" },
      )
      .toBe(2);

    // 3. Enable preview-on-click and open the kanban board.
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const previewCard = kanban.taskCardByTitle("Preview Tabs Task");
    await expect(previewCard).toBeVisible({ timeout: 10_000 });
    await expect(previewCard.getByRole("button", { name: "Open full page" })).toBeVisible({
      timeout: 10_000,
    });
    await previewCard.click();

    // 4. Preview panel + both tabs are visible.
    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });

    const primaryTab = testPage.getByTestId(`preview-session-tab-${primaryId}`);
    const secondaryTab = testPage.getByTestId(`preview-session-tab-${secondaryId}`);
    await expect(primaryTab).toBeVisible({ timeout: 10_000 });
    await expect(secondaryTab).toBeVisible();

    // 5. Primary tab is active by default and its session content is visible.
    // "simple mock response" appears only in the agent's reply, not in any prompt,
    // so the single getByText match is unambiguous.
    await expect(primaryTab).toHaveAttribute("data-state", "active");
    await expect(secondaryTab).toHaveAttribute("data-state", "inactive");
    await expect(previewPanel.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 6. Click the secondary tab → content switches, URL updates.
    await secondaryTab.click();
    await expect(secondaryTab).toHaveAttribute("data-state", "active");
    await expect(primaryTab).toHaveAttribute("data-state", "inactive");
    await expect(
      previewPanel.getByText("secondary-session-response", { exact: false }).first(),
    ).toBeVisible({ timeout: 15_000 });
    await expect(
      previewPanel.getByText("simple mock response", { exact: false }),
    ).not.toBeVisible();
    await expect(testPage).toHaveURL(new RegExp(`sessionId=${secondaryId}`), { timeout: 5_000 });

    // 9. Read-only tab bar: no close buttons and no add button are rendered.
    await expect(testPage.getByTestId(`preview-session-tab-close-${primaryId}`)).toHaveCount(0);
    await expect(testPage.getByTestId(`preview-session-tab-close-${secondaryId}`)).toHaveCount(0);
    await expect(previewPanel.getByRole("button", { name: "+" })).toHaveCount(0);
  });
});

/**
 * Verifies the lazy-workspace-setup behavior: opening the kanban preview for
 * a task with no sessions auto-launches one (using the workspace default agent
 * profile) so the user lands on a usable agent tab instead of the
 * "No agents yet." dead-end.
 */
test.describe("Preview auto-prepare", () => {
  test("auto-starts a session when previewing a task with no sessions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // 1. Make sure the workspace has a default agent profile so the preview
    //    can resolve one to start. The seed creates an agent profile but
    //    doesn't necessarily wire it as the workspace default.
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_agent_profile_id: seedData.agentProfileId,
    });

    // 2. Create a task with NO agent — it lands on the kanban with 0 sessions.
    const task = await apiClient.createTask(seedData.workspaceId, "Auto Prepare Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    // Sanity-check the precondition: the freshly created task must have no
    // sessions. Otherwise the "auto-prepare" path is never exercised.
    const before = await apiClient.listTaskSessions(task.id);
    expect(before.sessions ?? []).toHaveLength(0);

    // 3. Enable preview-on-click and open the kanban.
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();

    // 4. Preview panel renders. The empty "No agents yet." state must NOT
    //    appear at any point — the user should see "Preparing workspace…"
    //    bridging the gap and then the session tab.
    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });
    await expect(previewPanel.getByTestId("preview-empty-state")).toHaveCount(0);

    // 5. Eventually a session tab appears for the auto-started session.
    const sessionTab = previewPanel.locator('[data-testid^="preview-session-tab-"]');
    await expect(sessionTab.first()).toBeVisible({ timeout: 30_000 });

    // 6. The auto-launched session is reflected in the backend.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.length;
        },
        { timeout: 30_000, message: "Waiting for auto-prepared session to be created" },
      )
      .toBeGreaterThan(0);
  });

  // Regression test for the snapshot/PR-review case: tasks that don't carry
  // their own metadata.agent_profile_id used to dead-end on "No agents yet."
  // The resolver now also walks the workflow step → workflow chain, so a step
  // with its own agent_profile_id is enough to auto-start even when the task
  // and workspace have nothing set.
  test("auto-starts using the workflow step's agent_profile_id when task has none", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // 1. Create a second agent profile distinct from the seeded one so we can
    //    prove the resolver picked the step's profile (not the workspace
    //    default that the previous test in this file may have left set).
    const { agents } = await apiClient.listAgents();
    const stepProfile = await apiClient.createAgentProfile(agents[0].id, "Step Profile", {
      model: "mock-fast",
    });

    // 2. Pin that profile on the start step. The workspace default is left
    //    alone — whether or not it is set, the step value must win.
    await apiClient.updateWorkflowStep(seedData.startStepId, {
      agent_profile_id: stepProfile.id,
    });

    // 3. Task with NO agent and NO metadata override — the only place a
    //    profile can come from is the step.
    const task = await apiClient.createTask(seedData.workspaceId, "Step Profile Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const before = await apiClient.listTaskSessions(task.id);
    expect(before.sessions ?? []).toHaveLength(0);

    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();

    // 4. Preview panel opens and skips the empty state.
    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });
    await expect(previewPanel.getByTestId("preview-empty-state")).toHaveCount(0);

    // 5. A session tab appears for the auto-started session.
    const sessionTab = previewPanel.locator('[data-testid^="preview-session-tab-"]');
    await expect(sessionTab.first()).toBeVisible({ timeout: 30_000 });

    // 6. The auto-launched session uses the STEP's profile, not the workspace
    //    default — this is the regression-bait assertion that proves the
    //    backend session.ensure resolution chain (task metadata → step → workflow
    //    → workspace default) honors the step override.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions[0]?.agent_profile_id ?? null;
        },
        { timeout: 30_000, message: "Waiting for session created with step's profile" },
      )
      .toBe(stepProfile.id);

    // Restore the workflow step's agent_profile_id to null so subsequent
    // tests don't inherit a stale step-level override. The per-test
    // cleanupTestProfiles deletes stepProfile, but it doesn't touch the
    // workflow step itself; without this reset the next test creates a
    // task whose session resolves the (now-deleted) stepProfile.id and
    // fails with "agent profile not found".
    await apiClient
      .updateWorkflowStep(seedData.startStepId, { agent_profile_id: "" })
      .catch(() => undefined);
  });
});
