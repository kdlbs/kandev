import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

type SeedOpts = {
  apiClient: ApiClient;
  workspaceId: string;
  agentProfileId: string;
  repositoryId: string;
  title: string;
  currentUser: string;
};

async function seedApproveTest(opts: SeedOpts) {
  const { apiClient, workspaceId, agentProfileId, repositoryId, title, currentUser } = opts;
  const workflow = await apiClient.createWorkflow(workspaceId, `${title} Workflow`);
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
    workspace_id: workspaceId,
    workflow_filter_id: workflow.id,
    enable_preview_on_click: false,
  });

  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser(currentUser);

  const task = await apiClient.createTask(workspaceId, title, {
    workflow_id: workflow.id,
    workflow_step_id: inboxStep.id,
    agent_profile_id: agentProfileId,
    repository_ids: [repositoryId],
  });

  return { workflow, workingStep, doneStep, task };
}

async function openTaskAndPRPanel(testPage: Page, doneStepId: string, title: string) {
  const kanban = new KanbanPage(testPage);
  await kanban.taskCardInColumn(title, doneStepId).click();
  await expect(testPage).toHaveURL(/\/[st]\//, { timeout: 15_000 });
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
  await session.prTopbarButton().click();
  await expect(session.prDetailPanel()).toBeVisible({ timeout: 10_000 });
  return session;
}

test.describe("PR Approve button visibility", () => {
  /**
   * Regression: the Approve PR button must be hidden when the current GitHub
   * user is the author of the PR — GitHub's API rejects self-approval, so the
   * button would only ever produce a failed request.
   */
  test("is hidden when current user authored the PR", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);

    const { workflow, workingStep, doneStep, task } = await seedApproveTest({
      apiClient,
      workspaceId: seedData.workspaceId,
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
      title: "Self-Authored PR",
      currentUser: "test-user",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.moveTask(task.id, workflow.id, workingStep.id);

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 201,
      pr_url: "https://github.com/testorg/testrepo/pull/201",
      pr_title: "My own PR",
      head_branch: "feat/self",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });

    await expect(kanban.taskCardInColumn("Self-Authored PR", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    const session = await openTaskAndPRPanel(testPage, doneStep.id, "Self-Authored PR");
    await expect(session.prApproveButton()).toBeHidden();
  });

  /**
   * Control: when the PR is authored by someone else, the Approve button
   * must render so the user can submit an approval.
   */
  test("is visible when PR is authored by another user", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { workflow, workingStep, doneStep, task } = await seedApproveTest({
      apiClient,
      workspaceId: seedData.workspaceId,
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
      title: "Other-Authored PR",
      currentUser: "test-user",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.moveTask(task.id, workflow.id, workingStep.id);

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 202,
      pr_url: "https://github.com/testorg/testrepo/pull/202",
      pr_title: "Someone else's PR",
      head_branch: "feat/other",
      base_branch: "main",
      author_login: "someone-else",
      state: "open",
    });

    await expect(kanban.taskCardInColumn("Other-Authored PR", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    const session = await openTaskAndPRPanel(testPage, doneStep.id, "Other-Authored PR");
    await expect(session.prApproveButton()).toBeVisible({ timeout: 10_000 });
  });
});
