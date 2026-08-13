import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

const OWNER = "acme";
const REPO = "demo";

type SeedResult = {
  doneStepId: string;
  taskId: string;
};

/**
 * Stand up a workspace + workflow + completed task, mirroring
 * pr-topbar-popover.spec.ts: these tests exercise the PR popover, so the
 * session is seeded directly instead of launching an agent.
 */
async function seedTask(
  apiClient: ApiClient,
  workspaceId: string,
  agentProfileId: string,
  repositoryId: string,
  title: string,
): Promise<SeedResult> {
  const workflow = await apiClient.createWorkflow(workspaceId, `${title} Workflow`);
  await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
  const done = await apiClient.createWorkflowStep(workflow.id, "Done", 1);

  await apiClient.saveUserSettings({
    workspace_id: workspaceId,
    workflow_filter_id: workflow.id,
    enable_preview_on_click: false,
  });

  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");

  const task = await apiClient.createTask(workspaceId, title, {
    workflow_id: workflow.id,
    workflow_step_id: done.id,
    agent_profile_id: agentProfileId,
    repository_ids: [repositoryId],
  });
  const now = new Date().toISOString();
  await apiClient.seedTaskSession(task.id, {
    state: "COMPLETED",
    agentProfileId,
    repositoryId,
    startedAt: now,
    completedAt: now,
  });

  return { doneStepId: done.id, taskId: task.id };
}

async function openTaskAndWait(
  testPage: import("@playwright/test").Page,
  seed: SeedResult,
  title: string,
): Promise<SessionPage> {
  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await expect(kanban.taskCardInColumn(title, seed.doneStepId)).toBeVisible({ timeout: 15_000 });
  await kanban.taskCardInColumn(title, seed.doneStepId).click();
  await expect(testPage).toHaveURL(/\/[st]\//, { timeout: 15_000 });
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
  return session;
}

test.describe("PR disposition control", () => {
  test("records superseded with a URL on a closed, unmerged PR; the value survives reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Disposition Closed Unmerged";
    const prNumber = 501;
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: seed.taskId,
      owner: OWNER,
      repo: REPO,
      pr_number: prNumber,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${prNumber}`,
      pr_title: "Closed unmerged PR",
      head_branch: "feat/superseded",
      base_branch: "main",
      author_login: "test-user",
      state: "closed",
    });

    const session = await openTaskAndWait(testPage, seed, title);
    await session.hoverPRTopbar();

    const dispositionRow = session.prTopbarPopover().getByTestId("pr-disposition-row");
    await expect(dispositionRow).toBeVisible({ timeout: 10_000 });

    await dispositionRow.getByTestId("pr-disposition-select").click();
    await testPage.getByRole("listbox").getByRole("option", { name: "Superseded" }).click();

    const urlInput = dispositionRow.getByTestId("pr-disposition-superseded-url");
    await expect(urlInput).toBeVisible();
    await urlInput.fill(`https://github.com/${OWNER}/${REPO}/pull/502`);
    await dispositionRow.getByTestId("pr-disposition-save").click();

    await expect(dispositionRow.getByTestId("pr-disposition-select")).toContainText("Superseded", {
      timeout: 10_000,
    });

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await session.hoverPRTopbar();
    const dispositionRowAfterReload = session.prTopbarPopover().getByTestId("pr-disposition-row");
    await expect(dispositionRowAfterReload).toBeVisible({ timeout: 10_000 });
    await expect(dispositionRowAfterReload.getByTestId("pr-disposition-select")).toContainText(
      "Superseded",
    );
  });

  test("offers no disposition control for a merged PR", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Disposition Merged";
    const prNumber = 503;
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: seed.taskId,
      owner: OWNER,
      repo: REPO,
      pr_number: prNumber,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${prNumber}`,
      pr_title: "Merged PR",
      head_branch: "feat/merged",
      base_branch: "main",
      author_login: "test-user",
      state: "merged",
    });

    const session = await openTaskAndWait(testPage, seed, title);
    await session.hoverPRTopbar();
    await expect(session.prTopbarPopover()).toBeVisible({ timeout: 10_000 });
    await expect(session.prTopbarPopover().getByTestId("pr-disposition-row")).toHaveCount(0);
  });
});
