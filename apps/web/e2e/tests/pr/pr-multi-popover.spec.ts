import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

const OWNER = "acme";

type SeedResult = {
  workflowId: string;
  workingStepId: string;
  doneStepId: string;
  taskId: string;
};

/**
 * Stand up a workspace + workflow + task that reaches the Done column
 * immediately (auto-start + on_turn_complete moves it), mirroring
 * pr-topbar-popover.spec.ts.
 */
async function seedTask(
  apiClient: ApiClient,
  workspaceId: string,
  agentProfileId: string,
  repositoryId: string,
  title: string,
): Promise<SeedResult> {
  const workflow = await apiClient.createWorkflow(workspaceId, `${title} Workflow`);
  const inbox = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
  const working = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
  const done = await apiClient.createWorkflowStep(workflow.id, "Done", 2);

  await apiClient.updateWorkflowStep(working.id, {
    prompt: 'e2e:message("done")\n{{task_prompt}}',
    events: {
      on_enter: [{ type: "auto_start_agent" }],
      on_turn_complete: [{ type: "move_to_step", config: { step_id: done.id } }],
    },
  });

  await apiClient.saveUserSettings({
    workspace_id: workspaceId,
    workflow_filter_id: workflow.id,
    enable_preview_on_click: false,
  });

  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");

  const task = await apiClient.createTask(workspaceId, title, {
    workflow_id: workflow.id,
    workflow_step_id: inbox.id,
    agent_profile_id: agentProfileId,
    repository_ids: [repositoryId],
  });

  return {
    workflowId: workflow.id,
    workingStepId: working.id,
    doneStepId: done.id,
    taskId: task.id,
  };
}

/** Associate two open PRs with the task: web#42 failing, api#77 all green. */
async function associateTwoPRs(apiClient: ApiClient, taskId: string) {
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: taskId,
    owner: OWNER,
    repo: "web",
    pr_number: 42,
    pr_url: `https://github.com/${OWNER}/web/pull/42`,
    pr_title: "Failing web PR",
    head_branch: "feat/web",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    checks_state: "failure",
    checks_total: 6,
    checks_passing: 4,
    review_state: "changes_requested",
    review_count: 1,
  });
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: taskId,
    owner: OWNER,
    repo: "api",
    pr_number: 77,
    pr_url: `https://github.com/${OWNER}/api/pull/77`,
    pr_title: "Green api PR",
    head_branch: "feat/api",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    checks_state: "success",
    checks_total: 4,
    checks_passing: 4,
    review_state: "approved",
    review_count: 2,
    required_reviews: 2,
    mergeable_state: "clean",
  });
}

async function openTaskAndWait(
  testPage: import("@playwright/test").Page,
  apiClient: ApiClient,
  seed: SeedResult,
  title: string,
): Promise<SessionPage> {
  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await apiClient.moveTask(seed.taskId, seed.workflowId, seed.workingStepId);
  await expect(kanban.taskCardInColumn(title, seed.doneStepId)).toBeVisible({ timeout: 45_000 });
  await kanban.taskCardInColumn(title, seed.doneStepId).click();
  await expect(testPage).toHaveURL(/\/[st]\//, { timeout: 15_000 });
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
  return session;
}

test.describe("Multi-PR CI popover", () => {
  test("hover opens tabbed popover defaulting to the worst-status PR; tab switch swaps CI detail", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Multi Popover Tabs";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associateTwoPRs(apiClient, seed.taskId);
    const session = await openTaskAndWait(testPage, apiClient, seed, title);

    await expect(session.prTopbarButton()).toHaveAttribute("data-pr-count", "2");
    await session.hoverPRTopbar();
    await expect(session.prTopbarPopoverAggregate()).toBeVisible({ timeout: 10_000 });

    // Default tab = worst status: failing web#42, not first-created green api#77.
    await expect(session.prMultiPopoverTab("web", 42)).toHaveAttribute("data-active", "true");
    await expect(session.prMultiPopoverTab("api", 77)).toHaveAttribute("data-active", "false");
    // Body shows the failing PR's bucket groups (failed group present).
    await expect(session.prCheckGroup("failed")).toBeVisible();

    // Switch to the green PR: failed group disappears, passed shows 4.
    await session.prMultiPopoverTab("api", 77).click();
    await expect(session.prMultiPopoverTab("api", 77)).toHaveAttribute("data-active", "true");
    await expect(session.prCheckGroup("failed")).toHaveCount(0);
    await expect(session.prCheckGroupCount("passed")).toHaveText("4");
  });

  test("chat-input chip aggregates multiple PRs and opens the tabbed popover", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Multi Popover Chip";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associateTwoPRs(apiClient, seed.taskId);
    const session = await openTaskAndWait(testPage, apiClient, seed, title);

    const chip = session.prStatusChip();
    await expect(chip).toBeVisible();
    await expect(chip).toHaveAttribute("data-pr-count", "2");
    // web#42 is failing, so the aggregate (worst-of) status is failed.
    await expect(chip).toHaveAttribute("data-status", "failed");

    await chip.hover();
    await expect(session.prTopbarPopoverAggregate()).toBeVisible({ timeout: 10_000 });
    await expect(session.prMultiPopoverTab("web", 42)).toHaveAttribute("data-active", "true");
  });

  test("clicking the multi-PR button still opens the dropdown with per-PR rows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Multi Popover Dropdown";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associateTwoPRs(apiClient, seed.taskId);
    const session = await openTaskAndWait(testPage, apiClient, seed, title);

    await session.prTopbarButton().click();
    await expect(testPage.getByTestId("pr-topbar-menu-item-42")).toBeVisible();
    await expect(testPage.getByTestId("pr-topbar-menu-item-77")).toBeVisible();
  });

  test('selecting a different PR from the "+" add-panel menu opens its own tab', async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Multi Popover Add Panel Tabs";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associateTwoPRs(apiClient, seed.taskId);
    const session = await openTaskAndWait(testPage, apiClient, seed, title);

    // Auto-shown panel defaults to the primary/first-associated PR (web#42).
    await expect(session.prDetailTab()).toHaveCount(1, { timeout: 15_000 });

    // Regression: selecting the OTHER PR from the "+" add-panel menu must
    // open a second, distinct tab instead of repurposing the auto-shown one.
    // (Dedup when re-selecting the same PR is covered by the
    // runAutoPRPanelEffect / addPRPanel unit tests.)
    await session.addPanelButton().click();
    await testPage.getByTestId(`add-panel-pr-item-${OWNER}-api-77`).click();
    await expect(session.prDetailTab()).toHaveCount(2, { timeout: 15_000 });
  });
});
