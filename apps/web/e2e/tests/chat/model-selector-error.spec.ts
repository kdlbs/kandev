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
    await session.waitForChatIdle({ timeout: 30_000 });

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
    await session.waitForChatIdle({ timeout: 30_000 });

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
    // Resolves once the first (stale) request has fully completed its
    // route.fulfill, so the test can deterministically await propagation
    // instead of using a fixed sleep.
    let firstSettled: (() => void) | null = null;
    const firstSettledPromise = new Promise<void>((resolve) => {
      firstSettled = resolve;
    });
    let callCount = 0;
    await testPage.route("**/set-config-option", async (route) => {
      callCount += 1;
      if (callCount === 1) {
        await firstHeld;
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "stale failure" }),
        });
        firstSettled?.();
        return;
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
    await firstSettledPromise;
    await expect(testPage.getByTestId("toast-message")).toHaveCount(0);
    // Trigger should still reflect the newer (successful) selection.
    await expect(trigger).toContainText("Mock Fast", { timeout: 5_000 });
  });
});

/**
 * Verifies that a model change selected via the chat-input model selector
 * survives a full page reload. The regression this guards against: the
 * SetConfigOption RPC path used by mock-agent did not emit the session_models
 * convergence event, so the orchestrator never persisted the new model to the
 * session snapshot and SSR re-served the pre-change model on reload.
 */
test.describe("Chat model selector — persistence", () => {
  test("model change persists across page reload", async ({ testPage, apiClient, seedData }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Model Selector Persistence Test",
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
    await session.waitForChatIdle({ timeout: 30_000 });

    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast", { timeout: 15_000 });

    // Switch from Mock Fast to Mock Smart. mock-agent routes model changes
    // through POST /set-config-option — the same path the persistence bug
    // lived on (SetConfigOption did not emit the session_models convergence
    // event, so the orchestrator never wrote the new model to the DB).
    await trigger.click();
    await testPage.getByRole("option", { name: /Mock Smart/ }).click();
    await expect(trigger).toContainText("Mock Smart", { timeout: 5_000 });

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // After reload the trigger label comes from SSR-hydrated session state.
    // If SetConfigOption fails to emit the convergence event, the orchestrator
    // never persists the change and SSR re-serves "Mock Fast" — this assertion
    // is the regression guard.
    await expect(trigger).toContainText("Mock Smart", { timeout: 15_000 });
    await expect(trigger).not.toContainText("Mock Fast");
  });
});

/**
 * Verifies the chat-input model selector's popover stays open after picking a
 * model when extra config options (reasoning effort, thought level, …) are
 * present in the same selector. Picking a model is rarely the last thing a user
 * wants to do when there are other knobs available — they typically want to
 * adjust effort too. Auto-closing on model select forces a second open.
 *
 * mock-agent ships with model + effort config options, so the popover renders
 * an Effort row below the model list and opens the Effort picker from there.
 */
test.describe("Chat model selector — popover open/close behavior", () => {
  test("stays open after picking a model when extra config options exist", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Model Selector Stay Open Test",
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
    await session.waitForChatIdle({ timeout: 30_000 });

    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast", { timeout: 15_000 });

    await trigger.click();
    // The effort config row is the rendered marker that the popover content is
    // mounted. It opens the nested effort selector inside the same popover.
    const effortTrigger = testPage.getByTestId("config-option-trigger-effort");
    await expect(effortTrigger).toBeVisible({ timeout: 5_000 });

    await testPage.getByRole("option", { name: /Mock Smart/ }).click();

    // The trigger label updates optimistically — wait for that so we know the
    // selection round-tripped through the handler.
    await expect(trigger).toContainText("Mock Smart", { timeout: 5_000 });

    // Popover must still be open so the user can also pick an effort level
    // without re-opening. The effort row is rendered only while PopoverContent
    // is mounted (Radix unmounts it on close).
    const updatedEffortTrigger = testPage.getByTestId("config-option-trigger-effort");
    await expect(updatedEffortTrigger).toBeVisible();
  });
});
