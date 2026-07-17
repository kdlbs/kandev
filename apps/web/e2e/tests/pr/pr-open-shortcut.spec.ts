import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

const OWNER = "acme";
const SHORTCUT = "ControlOrMeta+Shift+G";

type SeedResult = {
  workflowId: string;
  workingStepId: string;
  doneStepId: string;
  taskId: string;
};

/**
 * Stand up a workspace + workflow + task that reaches the Done column
 * immediately (auto-start + on_turn_complete moves it), mirroring
 * pr-multi-popover.spec.ts.
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

async function associatePR(
  apiClient: ApiClient,
  taskId: string,
  repo: string,
  prNumber: number,
  title: string,
) {
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: taskId,
    owner: OWNER,
    repo,
    pr_number: prNumber,
    pr_url: `https://github.com/${OWNER}/${repo}/pull/${prNumber}`,
    pr_title: title,
    head_branch: `feat/${repo}`,
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    checks_state: "success",
    checks_total: 4,
    checks_passing: 4,
    review_state: "approved",
    review_count: 1,
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

test.describe("Open-task-PR keyboard shortcut", () => {
  test("Cmd+Shift+G with several linked PRs opens the picker; Enter opens the focused PR", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const title = "PR Shortcut Picker";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, "web", 42, "Web feature PR");
    await associatePR(apiClient, seed.taskId, "api", 77, "API feature PR");
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await expect(session.prTopbarButton()).toHaveAttribute("data-pr-count", "2");

    // Keep the picked PR's navigation offline-deterministic.
    await testPage
      .context()
      .route("https://github.com/**", (route) =>
        route.fulfill({ contentType: "text/html", body: "<html>github stub</html>" }),
      );

    await testPage.keyboard.press(SHORTCUT);
    const list = testPage.getByTestId("task-pr-picker-list");
    await expect(list).toBeVisible();
    await expect(list.locator("button[data-pr-row]")).toHaveCount(2);
    await prCapture.screenshot("pr-picker-modal", {
      caption: "Cmd+Shift+G on a task with two linked PRs opens the picker modal",
    });

    // First row (web#42) is auto-focused; ArrowDown moves to api#77, Enter opens it.
    await testPage.keyboard.press("ArrowDown");
    const popupPromise = testPage.context().waitForEvent("page");
    await testPage.keyboard.press("Enter");
    const popup = await popupPromise;
    await popup.waitForURL(`https://github.com/${OWNER}/api/pull/77`, { timeout: 10_000 });
    await expect(list).not.toBeVisible();
    await popup.close();
  });

  test("Cmd+Shift+G with one linked PR opens it directly without a modal", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "PR Shortcut Direct";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, "web", 42, "Web feature PR");
    await openTaskAndWait(testPage, apiClient, seed, title);

    await testPage
      .context()
      .route("https://github.com/**", (route) =>
        route.fulfill({ contentType: "text/html", body: "<html>github stub</html>" }),
      );

    const popupPromise = testPage.context().waitForEvent("page");
    await testPage.keyboard.press(SHORTCUT);
    const popup = await popupPromise;
    await popup.waitForURL(`https://github.com/${OWNER}/web/pull/42`, { timeout: 10_000 });
    await expect(testPage.getByTestId("task-pr-picker-list")).not.toBeVisible();
    await popup.close();
  });

  test("shortcut is rebindable from the general settings page", async ({ testPage, prCapture }) => {
    await testPage.goto("/settings/general/keyboard-shortcuts");
    const recorder = testPage.getByTestId("shortcut-recorder-OPEN_TASK_PR");
    await recorder.scrollIntoViewIfNeeded();
    await expect(recorder).toBeVisible();
    await prCapture.screenshot("settings-shortcut-row", {
      caption: "The shortcut is rebindable in Settings — Keyboard Shortcuts",
    });
  });
});
