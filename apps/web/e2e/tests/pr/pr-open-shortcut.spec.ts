import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import { GITLAB_HOST, GITLAB_PROJECT, seedGitLabReview } from "../../helpers/gitlab";
import type { Page } from "@playwright/test";

const OWNER = "acme";
const SHORTCUT = "ControlOrMeta+Shift+G";

type SeedResult = {
  workflowId: string;
  workingStepId: string;
  doneStepId: string;
  taskId: string;
};

type AssociateMRArgs = {
  apiClient: ApiClient;
  workspaceId: string;
  taskId: string;
  repositoryId: string;
  iid: number;
  title: string;
};

function taskReviewModifier() {
  return process.platform === "darwin" ? "Meta" : "Control";
}

async function associateMR({
  apiClient,
  workspaceId,
  taskId,
  repositoryId,
  iid,
  title,
}: AssociateMRArgs): Promise<string> {
  await seedGitLabReview(apiClient, workspaceId, iid, title);
  await apiClient.updateRepository(repositoryId, {
    provider: "gitlab",
    provider_host: GITLAB_HOST,
    provider_owner: "platform",
    provider_name: "kandev",
  });
  const mrURL = `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`;
  await apiClient.linkTaskGitLabMR(workspaceId, {
    task_id: taskId,
    repository_id: repositoryId,
    mr_url: mrURL,
  });
  return mrURL;
}

async function stubExternalProviders(page: Page) {
  await page
    .context()
    .route("https://github.com/**", (route) =>
      route.fulfill({ contentType: "text/html", body: "<html>github stub</html>" }),
    );
  await page
    .context()
    .route(`${GITLAB_HOST}/**`, (route) =>
      route.fulfill({ contentType: "text/html", body: "<html>gitlab stub</html>" }),
    );
}

async function holdTaskReviewShortcut(page: Page): Promise<string> {
  const modifier = taskReviewModifier();
  await page.keyboard.down(modifier);
  await page.keyboard.down("Shift");
  await page.keyboard.press("g");
  return modifier;
}

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
  test("Cmd+Shift+G cycles linked PRs and MRs until primary modifier release", async ({
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
    const mrURL = await associateMR({
      apiClient,
      workspaceId: seedData.workspaceId,
      taskId: seed.taskId,
      repositoryId: seedData.repositoryId,
      iid: 88,
      title: "GitLab linked review",
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await expect(session.prTopbarButton()).toHaveAttribute("data-pr-count", "2");

    await stubExternalProviders(testPage);

    const modifier = await holdTaskReviewShortcut(testPage);
    const list = testPage.getByTestId("task-pr-picker-list");
    await expect(list).toBeVisible();
    const rows = list.locator("button[data-pr-row]");
    const firstPR = rows.nth(0);
    const secondPR = rows.nth(1);
    const mergeRequest = rows.nth(2);
    await expect(rows).toHaveCount(3);
    await expect(firstPR).toHaveAttribute("data-selected", "true");
    await expect(mergeRequest).toContainText("GitLab linked review");
    await prCapture.screenshot("pr-picker-modal", {
      caption: "Cmd+Shift+G opens a held shortcut picker for linked code reviews",
    });
    if (process.env.CAPTURE_PR_ASSETS) {
      await testPage.setViewportSize({ width: 390, height: 844 });
      await prCapture.screenshot("mobile-pr-picker-modal", {
        caption: "Linked code review picker on a mobile viewport",
      });
      await testPage.setViewportSize({ width: 1280, height: 720 });
    }

    // Shift release alone does not commit; Ctrl/Cmd remains held for raw G cycling.
    const pageCountBeforeRelease = testPage.context().pages().length;
    await testPage.keyboard.up("Shift");
    await expect(list).toBeVisible();
    await expect(firstPR).toHaveAttribute("data-selected", "true");
    expect(testPage.context().pages()).toHaveLength(pageCountBeforeRelease);

    await testPage.keyboard.press("g");
    await expect(secondPR).toHaveAttribute("data-selected", "true");
    await testPage.keyboard.press("g");
    await expect(mergeRequest).toHaveAttribute("data-selected", "true");
    await testPage.keyboard.press("g");
    await expect(firstPR).toHaveAttribute("data-selected", "true");
    await testPage.keyboard.press("g");
    await expect(secondPR).toHaveAttribute("data-selected", "true");
    await testPage.keyboard.press("g");
    await expect(mergeRequest).toHaveAttribute("data-selected", "true");

    const popupPromise = testPage.context().waitForEvent("page");
    await testPage.keyboard.up(modifier);
    const popup = await popupPromise;
    await popup.waitForURL(mrURL, { timeout: 10_000 });
    await expect(list).not.toBeVisible();
    await popup.close();
  });

  test("Escape cancels a held picker without opening a review on release", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "PR Shortcut Cancel";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, "web", 42, "Web feature PR");
    await associatePR(apiClient, seed.taskId, "api", 77, "API feature PR");
    await openTaskAndWait(testPage, apiClient, seed, title);
    await stubExternalProviders(testPage);

    const modifier = await holdTaskReviewShortcut(testPage);
    const list = testPage.getByTestId("task-pr-picker-list");
    await expect(list).toBeVisible();

    await testPage.keyboard.press("Escape");
    await expect(list).not.toBeVisible();
    const pageCountBeforeRelease = testPage.context().pages().length;
    await testPage.keyboard.up("Shift");
    await testPage.keyboard.up(modifier);

    expect(testPage.context().pages()).toHaveLength(pageCountBeforeRelease);
    await expect(testPage).toHaveURL(new RegExp(`/[st]/${seed.taskId}`));
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

    await stubExternalProviders(testPage);

    const popupPromise = testPage.context().waitForEvent("page");
    await testPage.keyboard.press(SHORTCUT);
    const popup = await popupPromise;
    await popup.waitForURL(`https://github.com/${OWNER}/web/pull/42`, { timeout: 10_000 });
    await expect(testPage.getByTestId("task-pr-picker-list")).not.toBeVisible();
    await popup.close();
  });

  test("shortcut is rebindable from the general settings page", async ({ testPage, prCapture }) => {
    await testPage.goto("/settings/preferences/keyboard-shortcuts");
    const recorder = testPage.getByTestId("shortcut-recorder-OPEN_TASK_PR");
    await recorder.scrollIntoViewIfNeeded();
    await expect(recorder).toBeVisible();
    await prCapture.screenshot("settings-shortcut-row", {
      caption: "The shortcut is rebindable in Settings — Keyboard Shortcuts",
    });
  });
});
