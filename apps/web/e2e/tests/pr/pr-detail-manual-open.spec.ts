import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("PR detail panel — manual open", () => {
  /**
   * Regression: when the user dismisses the auto-shown PR panel and re-opens
   * it manually via the topbar PR button, the new panel must land as a tab
   * inside the session's dockview group — not as a separate split group.
   *
   * The manual path uses `addPRPanel(prKey)` which historically anchored on
   * the store's `centerGroupId`. That value can be stale across layout
   * transitions and produces the "PR opens in a split" bug captured in the
   * screenshot from the issue.
   *
   * Setup:
   *   Inbox → Working (auto_start, on_turn_complete → Done) → Done
   *   Task A (with PR #501)
   */
  test("places PR panel as a tab in the session group when opened from topbar", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "PR Manual Open Workflow",
    );

    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const workingStep = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    await apiClient.updateWorkflowStep(workingStep.id, {
      prompt: 'e2e:message("done")\n{{task_prompt}}',
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 501,
        title: "Migrate Order Basket API",
        state: "open",
        head_branch: "feat/order-basket",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 953,
        deletions: 20,
      },
    ]);

    const taskA = await apiClient.createTask(seedData.workspaceId, "Order Basket Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await apiClient.moveTask(taskA.id, workflow.id, workingStep.id);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskA.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 501,
      pr_url: "https://github.com/testorg/testrepo/pull/501",
      pr_title: "Migrate Order Basket API",
      head_branch: "feat/order-basket",
      base_branch: "main",
      author_login: "test-user",
      additions: 953,
      deletions: 20,
    });

    await expect(kanban.taskCardInColumn("Order Basket Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Open the task — the auto-show hook adds the legacy `pr-detail` panel.
    await kanban.taskCardInColumn("Order Basket Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.idleInput().waitFor({ state: "visible", timeout: 30_000 });
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });

    // Dismiss the auto-shown PR panel. The auto-show hook already marked it
    // as "offered" on add, so it won't re-add it — the next open path is the
    // manual one.
    await session.prDetailTab().hover();
    const closeBtn = session.prDetailTab().locator(".dv-default-tab-action");
    await closeBtn.click();
    await expect(session.prDetailTab()).not.toBeVisible({ timeout: 10_000 });

    // Click the topbar PR button — exercises addPRPanel(prKey, activeSessionId)
    // and creates a keyed `pr-detail|owner/repo/N` panel.
    await session.prTopbarButton().click();
    await expect(session.prDetailPanel()).toBeVisible({ timeout: 10_000 });
    await expect(session.prDetailTab()).toBeVisible({ timeout: 10_000 });

    // Invariant: PR panel is a tab in the session's dockview group, not a
    // split into a separate group. Matches keyed (pr-detail|...) panel id.
    await session.expectAnyPrPanelAndSessionShareGroup();
  });
});
