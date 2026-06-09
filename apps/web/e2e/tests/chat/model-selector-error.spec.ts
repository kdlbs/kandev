import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Verifies the chat-input model selector's failure path:
 * when the backend rejects a model change, the UI shows a toast carrying the
 * backend error message and the trigger label reverts to the previous model.
 *
 * mock-agent exposes "model" as a config option, so changing the model goes
 * through POST /set-config-option. Agents without a model config option use
 * POST /set-model — both routes are stubbed for resilience.
 */
test.describe("Chat model selector — RPC failure", () => {
  test("shows error toast and reverts trigger label when model change fails", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Model Selector Error Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toBeVisible({ timeout: 15_000 });
    // mock-agent ships with model="mock-fast" + effort="medium", so the
    // composite trigger label starts as "Mock Fast / Medium".
    await expect(trigger).toContainText("Mock Fast", { timeout: 15_000 });

    const backendErrorMessage = "mock backend rejected model change";
    const fail = (route: import("@playwright/test").Route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: backendErrorMessage }),
      });
    await testPage.route("**/set-model", fail);
    await testPage.route("**/set-config-option", fail);

    await trigger.click();
    const smartRow = testPage.getByRole("option", { name: /Mock Smart/ });
    await expect(smartRow).toBeVisible({ timeout: 5_000 });
    await smartRow.click();

    const toast = testPage
      .getByTestId("toast-message")
      .filter({ hasText: "Failed to change model" });
    await expect(toast).toBeVisible({ timeout: 5_000 });
    await expect(toast).toContainText(backendErrorMessage);

    // After revert the trigger label should match the original (model + extras).
    await expect(trigger).toContainText("Mock Fast", { timeout: 5_000 });
    await expect(trigger).not.toContainText("Mock Smart");
  });

  test("stale failure does not revert a newer successful change", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Model Selector Race Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast", { timeout: 15_000 });

    // Hold the first config-option request open so we can fire a second one
    // before the first settles. Resolve the first as a 500 after the second
    // has already been issued — the stale rejection must NOT clobber the
    // newer optimistic state.
    let releaseFirst: (() => void) | null = null;
    const firstHeld = new Promise<void>((resolve) => {
      releaseFirst = resolve;
    });
    let callCount = 0;
    await testPage.route("**/set-config-option", async (route) => {
      callCount += 1;
      if (callCount === 1) {
        await firstHeld;
        return route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "stale failure" }),
        });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true }),
      });
    });

    await trigger.click();
    await testPage.getByRole("option", { name: /Mock Smart/ }).click();
    // Re-open and pick again — second request will succeed. mock-agent only
    // ships two models (Mock Fast / Mock Smart), so we go back to Mock Fast.
    await trigger.click();
    await testPage.getByRole("option", { name: /Mock Fast/ }).click();

    // Now release the first (stale) request — its 500 rejection should be
    // swallowed (no toast).
    releaseFirst?.();

    // Give the stale rejection a beat to propagate, then assert no toast was
    // shown by the stale failure.
    await testPage.waitForTimeout(500);
    await expect(testPage.getByTestId("toast-message")).toHaveCount(0);
    // Trigger should still reflect the newer (successful) selection.
    await expect(trigger).toContainText("Mock Fast", { timeout: 5_000 });
  });
});
