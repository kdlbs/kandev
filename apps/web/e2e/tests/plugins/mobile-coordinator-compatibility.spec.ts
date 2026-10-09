import { randomUUID } from "node:crypto";
import { expect, test } from "../../fixtures/test-base";
import { installAndGrantCoordinator, uninstallCoordinator } from "./reference-coordinator-helpers";
import {
  installAndGrantObserver,
  OBSERVER_PLUGIN_ID,
  uninstallObserver,
} from "./observer-compatibility-helpers";

test("phone policy parity and unavailable capability", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(150_000);
  const taskIds: string[] = [];
  await installAndGrantCoordinator(testPage, apiClient, seedData.workspaceId);
  await installAndGrantObserver(testPage, apiClient, seedData.workspaceId);
  try {
    await apiClient
      .createTask(seedData.workspaceId, `Phone observer source ${randomUUID().slice(0, 6)}`, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      })
      .then((task) => taskIds.push(task.id));

    await testPage.goto(`/plugins/${OBSERVER_PLUGIN_ID}`);
    const policy = testPage.getByTestId("observer-policy");
    await expect(policy).toContainText(
      "does not adopt tasks, start agents, or manage conversations",
    );
    const task = testPage.getByTestId("observer-task").first();
    await expect(task).toBeVisible();
    const propose = task.getByRole("button", { name: "Propose follow-up" });
    await expect(propose).toHaveCSS("min-height", "44px");
    await propose.tap();
    await testPage.getByTestId("observer-proposal-title").fill("Phone approved work");
    await testPage
      .getByTestId("observer-proposal-description")
      .fill("Keep the decision visible on a narrow screen.");
    const save = testPage.getByTestId("observer-proposal-submit");
    await expect(save).toHaveCSS("min-height", "44px");
    await save.tap();

    const proposal = testPage.getByTestId("observer-proposal");
    await expect(proposal).toContainText("Phone approved work");
    const beforeApproval = await apiClient.listTasks(seedData.workspaceId);
    expect(beforeApproval.tasks.some((item) => item.title === "Phone approved work")).toBe(false);

    const approve = proposal.getByRole("button", { name: "Approve and create task" });
    await expect(approve).toHaveCSS("min-height", "44px");
    await approve.tap();
    await expect(proposal).toContainText("agent not started");
    const createdTasks = await apiClient.listTasks(seedData.workspaceId);
    const createdTask = createdTasks.tasks.find((item) => item.title === "Phone approved work");
    expect(createdTask).toBeDefined();
    if (!createdTask) throw new Error("Phone approval did not create the proposed Host task");
    taskIds.push(createdTask.id);
    expect((await apiClient.listTaskSessions(createdTask.id)).sessions).toHaveLength(0);

    const geometry = await testPage.getByTestId("observer-page").evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }));
    expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.clientWidth + 1);
  } finally {
    for (const taskId of taskIds) await apiClient.deleteTask(taskId).catch(() => undefined);
    await uninstallObserver(apiClient);
    await uninstallCoordinator(apiClient);
  }
});
