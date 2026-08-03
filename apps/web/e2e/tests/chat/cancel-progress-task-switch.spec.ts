import { test, expect } from "../../fixtures/test-base";
import { waitForSessionDone, seedIdleSession } from "../../helpers/session";
import { waitForActiveSessionCancellationPending } from "../../helpers/session-store";

test.describe("Cancel progress across task switches", () => {
  test("keeps backend-owned cancel progress across task switches and reloads", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Cancel progress B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!taskB.session_id) throw new Error("Task B did not return a session_id");
    await waitForSessionDone(
      apiClient,
      taskB.id,
      taskB.session_id,
      "Task B should finish its seed turn before switching to it",
    );

    const session = await seedIdleSession(testPage, apiClient, seedData, "Cancel progress A");
    await session.sendMessage("/slow 8s");

    const activeCancel = session.activeChat().getByTestId("cancel-agent-button");
    await expect(activeCancel).toBeVisible({ timeout: 15_000 });
    await activeCancel.click();
    await waitForActiveSessionCancellationPending(testPage, true);
    await expect(activeCancel).toBeDisabled();
    await expect(activeCancel.getByRole("status", { name: "Loading" })).toBeVisible();

    await expect(session.sidebar.getByText("Cancel progress B", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
    await session.clickTaskInSidebar("Cancel progress B");
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskB.id}$`));
    await session.showSessionContext();
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    await session.clickTaskInSidebar("Cancel progress A");
    await expect(testPage).not.toHaveURL(new RegExp(`/t/${taskB.id}$`));
    await session.showSessionContext();

    const remountedCancel = session.activeChat().getByTestId("cancel-agent-button");
    await expect(remountedCancel).toBeVisible({ timeout: 15_000 });
    await expect(remountedCancel).toBeDisabled();
    await expect(remountedCancel.getByRole("status", { name: "Loading" })).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    const reloadedCancel = session.activeChat().getByTestId("cancel-agent-button");
    await expect(reloadedCancel).toBeVisible({ timeout: 15_000 });
    await expect(reloadedCancel).toBeDisabled();
    await expect(reloadedCancel.getByRole("status", { name: "Loading" })).toBeVisible();

    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await waitForActiveSessionCancellationPending(testPage, false);
    await expect(reloadedCancel).not.toBeVisible({ timeout: 15_000 });
  });
});
