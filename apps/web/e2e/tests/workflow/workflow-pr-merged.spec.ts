import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

const PR_OWNER = "acme";
const PR_REPO = "widgets";

test.describe("Workflow PR lifecycle events", () => {
  test("moves a task to the configured step when its linked PR is merged", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Merged Workflow");
    const acceptStep = await apiClient.createWorkflowStep(workflow.id, "Acceptance", 0);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Merged Done", 1, {
      complete_task_on_enter: true,
    });
    await apiClient.updateWorkflowStep(acceptStep.id, {
      events: {
        on_pr_merged: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Task awaiting merge", {
      workflow_id: workflow.id,
      workflow_step_id: acceptStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: `pr-merged-${task.id}`,
      agentProfileId: seedData.agentProfileId,
    });

    const prNumber = 4554;
    const linkedPR = {
      task_id: task.id,
      workspace_id: seedData.workspaceId,
      owner: PR_OWNER,
      repo: PR_REPO,
      pr_number: prNumber,
      pr_url: `https://github.com/${PR_OWNER}/${PR_REPO}/pull/${prNumber}`,
      pr_title: "Ship the feature",
      head_branch: "feat/ship",
      base_branch: "main",
      author_login: "kandev-e2e",
    };
    await apiClient.mockGitHubAssociateTaskPR({ ...linkedPR, state: "open" });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardInColumn("Task awaiting merge", acceptStep.id)).toBeVisible();
    await expect(kanban.taskCardInColumn("Task awaiting merge", doneStep.id)).toHaveCount(0);

    const mergedAt = new Date().toISOString();
    await apiClient.mockGitHubAssociateTaskPR({
      ...linkedPR,
      state: "merged",
      merged_at: mergedAt,
      closed_at: mergedAt,
    });

    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id)
      .toBe(doneStep.id);
    await expect(kanban.taskCardInColumn("Task awaiting merge", doneStep.id)).toBeVisible();
    await expect.poll(async () => (await apiClient.getTask(task.id)).state).toBe("COMPLETED");
    await expect
      .poll(async () => {
        const { history } = await apiClient.listWorkflowHistory(sessionId);
        return (history ?? []).map((row) => [row.to_step_id, row.trigger]);
      })
      .toContainEqual([doneStep.id, "on_pr_merged"]);
  });

  test("configures PR merged and PR closed transitions in the step editor", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "PR Event Settings");
    const execStep = await apiClient.createWorkflowStep(workflow.id, "Execution", 0);
    const acceptStep = await apiClient.createWorkflowStep(workflow.id, "Acceptance", 1);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Shipped", 2);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("PR Event Settings");
    await expect(card).toBeVisible();
    await page.stepNodeByName(card, "Acceptance").click();

    await card.getByTestId(`${acceptStep.id}-pr-merged-help`).hover();
    await expect(
      testPage.getByText("Runs once when every pull request linked to the task is merged."),
    ).toBeVisible();

    await card.getByTestId(`${acceptStep.id}-pr-merged-transition-select`).click();
    await testPage.getByRole("option", { name: "Move to specific step" }).click();
    await card.getByTestId(`${acceptStep.id}-pr-merged-step-select`).click();
    await testPage.getByRole("option", { name: "Shipped" }).click();
    await expect(card.getByTestId(`${acceptStep.id}-pr-merged-step-select`)).toContainText(
      "Shipped",
    );

    await card.getByTestId(`${acceptStep.id}-pr-closed-transition-select`).click();
    await testPage.getByRole("option", { name: "Move to previous step" }).click();

    await page.saveChanges();
    const afterSave = await apiClient.listWorkflowSteps(workflow.id);
    const savedAccept = afterSave.steps.find((step) => step.id === acceptStep.id);
    expect(savedAccept?.events?.on_pr_merged).toEqual([
      { type: "move_to_step", config: { step_id: doneStep.id } },
    ]);
    expect(savedAccept?.events?.on_pr_closed).toEqual([{ type: "move_to_previous" }]);
    expect(
      afterSave.steps.find((step) => step.id === execStep.id)?.events?.on_pr_merged,
    ).toBeUndefined();

    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("PR Event Settings");
    await page.selectStep(reloadedCard, "Acceptance");
    await expect(reloadedCard.getByTestId(`${acceptStep.id}-pr-merged-step-select`)).toContainText(
      "Shipped",
    );
    await expect(
      reloadedCard.getByTestId(`${acceptStep.id}-pr-closed-transition-select`),
    ).toContainText("Move to previous step");
  });
});
