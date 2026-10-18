import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test("saves both PR event selectors and opens their help by touch", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Phone PR settings");
  await apiClient.createWorkflowStep(workflow.id, "Execution", 0);
  const accept = await apiClient.createWorkflowStep(workflow.id, "Acceptance", 1);
  const done = await apiClient.createWorkflowStep(workflow.id, "Shipped", 2);
  const settings = new WorkflowSettingsPage(testPage);
  await settings.goto(seedData.workspaceId);
  const card = await settings.findWorkflowCard(workflow.name);
  await settings.selectStep(card, "Acceptance", true);

  for (const [event, helpText] of [
    ["pr-merged", "Runs once when every pull request linked to the task is merged."],
    ["pr-closed", "Runs once when a pull request linked to the task is closed without merging."],
  ]) {
    const help = card.getByTestId(`${accept.id}-${event}-help`);
    const box = await help.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.width).toBeGreaterThanOrEqual(44);
    await help.tap();
    const drawer = testPage.getByRole("dialog");
    await expect(drawer).toContainText(helpText);
    await testPage.keyboard.press("Escape");
    await expect(drawer).not.toBeVisible();
  }

  await card.getByTestId(`${accept.id}-pr-merged-transition-select`).tap();
  await testPage.getByRole("option", { name: "Move to specific step" }).tap();
  await card.getByTestId(`${accept.id}-pr-merged-step-select`).tap();
  await testPage.getByRole("option", { name: "Shipped", exact: true }).tap();
  await card.getByTestId(`${accept.id}-pr-closed-transition-select`).tap();
  await testPage.getByRole("option", { name: "Move to previous step" }).tap();
  const unsaved = (await apiClient.listWorkflowSteps(workflow.id)).steps;
  expect(unsaved.find((step) => step.id === accept.id)?.events?.on_pr_merged).toBeUndefined();
  await settings.saveChanges(true);
  const saved = (await apiClient.listWorkflowSteps(workflow.id)).steps;
  expect(saved.find((step) => step.id === accept.id)?.events).toMatchObject({
    on_pr_merged: [{ type: "move_to_step", config: { step_id: done.id } }],
    on_pr_closed: [{ type: "move_to_previous" }],
  });
  await settings.goto(seedData.workspaceId);
  const reloaded = await settings.findWorkflowCard(workflow.name);
  await settings.selectStep(reloaded, "Acceptance", true);
  await expect(reloaded.getByTestId(`${accept.id}-pr-merged-step-select`)).toContainText("Shipped");
  await expect(reloaded.getByTestId(`${accept.id}-pr-closed-transition-select`)).toContainText(
    "Move to previous step",
  );
  await assertNoDocumentHorizontalOverflow(testPage, "phone PR event settings");
});

for (const outcome of ["merged", "closed"] as const) {
  test(`shows the ${outcome} PR workflow outcome on a phone`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, `Phone PR ${outcome}`);
    const exec = await apiClient.createWorkflowStep(workflow.id, "Execution", 0);
    const accept = await apiClient.createWorkflowStep(workflow.id, "Acceptance", 1);
    const done = await apiClient.createWorkflowStep(workflow.id, "Shipped", 2, {
      complete_task_on_enter: true,
    });
    await apiClient.updateWorkflowStep(accept.id, {
      events: {
        on_pr_merged: [{ type: "move_to_step", config: { step_id: done.id } }],
        on_pr_closed: [{ type: "move_to_previous" }],
      },
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });
    const task = await apiClient.createTask(seedData.workspaceId, `Phone task ${outcome}`, {
      workflow_id: workflow.id,
      workflow_step_id: accept.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const sessionTime = new Date().toISOString();
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: outcome === "merged" ? "COMPLETED" : "WAITING_FOR_INPUT",
      sessionId: `phone-pr-${task.id}`,
      agentProfileId: seedData.agentProfileId,
      startedAt: sessionTime,
      ...(outcome === "merged" ? { completedAt: sessionTime } : {}),
    });
    const linked = {
      task_id: task.id,
      workspace_id: seedData.workspaceId,
      owner: "acme",
      repo: "widgets",
      pr_number: 4559,
      pr_url: "https://github.com/acme/widgets/pull/4559",
      pr_title: "Phone workflow outcome",
      head_branch: "feat/phone",
      base_branch: "main",
      author_login: "kandev-e2e",
    };
    await apiClient.mockGitHubAssociateTaskPR({ ...linked, state: "open" });
    await testPage.goto("/tasks?group=workflow_step");
    await expect(
      testPage.getByTestId("tasks-list").getByText(`Phone task ${outcome}`, { exact: true }),
    ).toBeVisible();
    const at = new Date().toISOString();
    await apiClient.mockGitHubAssociateTaskPR({
      ...linked,
      state: outcome,
      closed_at: at,
      ...(outcome === "merged" ? { merged_at: at } : {}),
    });
    const target = outcome === "merged" ? done : exec;
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id)
      .toBe(target.id);
    if (outcome === "merged") {
      await expect.poll(async () => (await apiClient.getTask(task.id)).state).toBe("COMPLETED");
    } else {
      expect((await apiClient.getTask(task.id)).state).not.toBe("COMPLETED");
    }
    await expect
      .poll(async () => {
        const { history } = await apiClient.listWorkflowHistory(sessionId);
        return (history ?? []).map((row) => [row.to_step_id, row.trigger]);
      })
      .toContainEqual([target.id, `on_pr_${outcome}`]);
    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions.find((session) => session.id === sessionId)?.state).toBe(
      outcome === "merged" ? "COMPLETED" : "WAITING_FOR_INPUT",
    );
    await testPage.reload();
    const section = testPage.getByTestId("tasks-list-section").filter({ hasText: target.name });
    await expect(section.getByText(`Phone task ${outcome}`, { exact: true })).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "phone PR workflow outcome");
  });
}
