import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import {
  installSandboxSettingsFixture,
  enableSandboxSettingsFixture,
} from "../../helpers/execution-settings";
import { KanbanPage } from "../../pages/kanban-page";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

test("task policy draft is discarded on cancel and saved only with Update", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const mobile = testInfo.project.name === "mobile-chrome";
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent");
  if (!agent) throw new Error("Mock agent required");
  const profile = await apiClient.createAgentProfile(agent.id, "Task policy preview", {
    model: "mock-fast",
  });
  const { executors } = await apiClient.listExecutors();
  const local = executors.find((item) => item.type === "local");
  if (!local?.profiles?.[0]) throw new Error("Local executor profile required");
  const task = await apiClient.createTask(seedData.workspaceId, "Task policy draft", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  await apiClient.updateTaskMetadata(task.id, {
    executor_profile_id: local.profiles[0].id,
    agent_profile_id: profile.id,
  });
  await apiClient.updateTaskState(task.id, "IN_PROGRESS");
  try {
    const fixture = await installSandboxSettingsFixture(testPage, task.id, executors, {
      scope: "task",
      agentProfileId: profile.id,
      workspaceId: seedData.workspaceId,
      workflowStepId: seedData.startStepId,
    });
    const board = mobile ? new MobileKanbanPage(testPage) : new KanbanPage(testPage);
    await board.goto();
    await enableSandboxSettingsFixture(testPage);
    const open = async () => {
      if (mobile)
        await testPage
          .getByTestId(`task-card-${task.id}`)
          .getByRole("button", { name: "More options" })
          .click();
      else await (board as KanbanPage).openTaskActionsMenu(task.id);
      await testPage.getByRole("menuitem", { name: "Edit", exact: true }).click();
    };
    await open();
    const dialog = testPage.getByTestId("create-task-dialog");
    const section = dialog.getByTestId("task-sandbox-settings");
    const policy = section.getByLabel("Default policy", { exact: true });
    await expect(section).toBeVisible();
    const agentContext = section.getByLabel("Agent context", { exact: true });
    if (await agentContext.count()) await agentContext.selectOption(profile.id);
    const executor = section.getByLabel("Executor context", { exact: true });
    if (await executor.isEnabled()) await executor.selectOption(local.id);
    await expect(policy).toBeEnabled();
    await policy.selectOption("revision-alpha");
    expect(fixture.writes).toHaveLength(0);
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    expect(fixture.writes).toHaveLength(0);
    await open();
    await expect(policy).toHaveValue("__inherit__");
    if (await agentContext.count()) await agentContext.selectOption(profile.id);
    if (await executor.isEnabled()) await executor.selectOption(local.id);
    await policy.selectOption("revision-alpha");
    if (mobile) {
      expect((await policy.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
    }
    await section.screenshot({ path: testInfo.outputPath("task-policy-settings.png") });
    await dialog.getByRole("button", { name: "Update", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    expect(fixture.writes).toHaveLength(1);
    expect(fixture.writes[0]).toMatchObject({
      scope: "task",
      target_id: task.id,
      revision_id: "revision-alpha",
    });
  } finally {
    await apiClient.deleteTask(task.id);
    await apiClient.deleteAgentProfile(profile.id, true);
  }
});
