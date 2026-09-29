// The workspace copilot on the board and task pages (docs/plans/workspace-coordinator-p2/task-10-copilot-everywhere.md).
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToTask } from "../../../lib/links";

const COORDINATOR_READ = /\/coordinators\/[^/]+$/;

test.describe("Workspace copilot everywhere", () => {
  test("opens from the board and the task page with a page chip", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Everywhere Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    const task = await apiClient.createTask(seedData.workspaceId, "Everywhere task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto("/");
    const launcher = testPage.getByTestId("workspace-copilot-launcher");
    await expect(launcher).toBeVisible({ timeout: 15_000 });
    const read = waitForHttp(testPage, "GET", COORDINATOR_READ);
    await launcher.click();
    await read;
    const panel = testPage.getByTestId("workspace-copilot-panel");
    await expect(panel).toBeVisible();
    await expect(panel.getByTestId("workspace-copilot-chip-label")).toBeVisible();

    await panel.getByRole("button", { name: "Close" }).first().click();
    await expect(panel).not.toBeVisible();

    await testPage.goto(linkToTask(task.id));
    await expect(testPage.getByTestId("workspace-copilot-launcher")).toBeVisible({
      timeout: 15_000,
    });
  });

  test("takes the full screen on a phone", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 390, height: 667 });
    await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Phone Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto("/");
    const launcher = testPage.getByTestId("workspace-copilot-launcher");
    await expect(launcher).toBeVisible({ timeout: 15_000 });
    await launcher.click();
    const panel = testPage.getByTestId("workspace-copilot-panel");
    await expect(panel).toBeVisible();
    const box = await panel.boundingBox();
    expect(box?.width).toBe(390);
  });
});
