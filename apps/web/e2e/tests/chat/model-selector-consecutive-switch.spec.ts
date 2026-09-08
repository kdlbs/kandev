import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { watchWs, type WsWatcher } from "../../helpers/causal-waits";

/**
 * Guards consecutive model switches from the chat-input model selector.
 *
 * mock-agent re-advertises its whole config catalog on a model change (the
 * smart model publishes a different effort option set), which is what a real
 * ACP provider does. The regression this covers: after the first switch the
 * picker stopped accepting further selections — the trigger no longer opened
 * the list — until a prompt refreshed the session.
 */

type ModelPicker = {
  trigger: Locator;
  /**
   * Opens the picker from a known-closed state, selects a model, and returns
   * once the provider has converged on it. The label cannot settle before that
   * session.models_updated notification lands, so waiting on the frame removes
   * the round trip from the assertions that follow.
   */
  pick: (name: RegExp, modelId: string) => Promise<void>;
};

function modelPicker(page: Page, ws: WsWatcher, sessionId: string): ModelPicker {
  const trigger = page.getByRole("button", { name: "Session model settings" });
  const listbox = page.getByRole("listbox");
  return {
    trigger,
    pick: async (name, modelId) => {
      if (await listbox.isVisible()) {
        await page.keyboard.press("Escape");
        await expect(listbox).toBeHidden();
      }
      await trigger.click();
      await expect(listbox).toBeVisible();

      const converged = ws.waitForEvent("session.models_updated", {
        where: (payload) =>
          payload.session_id === sessionId && payload.current_model_id === modelId,
      });
      await listbox.getByRole("option", { name }).click();
      await converged;
    },
  };
}

test.describe("Chat model selector — consecutive switches", () => {
  test("applies a second and third model switch after the first", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ws = watchWs(testPage);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Consecutive Model Switch Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const sessionId = task.session_id;
    if (!sessionId) throw new Error("expected an auto-started session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    // The startup model event can precede the page's subscription, so read the
    // settled model from the backend rather than waiting on a frame that may
    // already be gone.
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const metadata = sessions.find((item) => item.id === sessionId)?.metadata;
        return (metadata?.runtime_config as { model?: string } | undefined)?.model;
      })
      .toBe("mock-fast");
    const { trigger, pick } = modelPicker(testPage, ws, sessionId);
    await expect(trigger).toContainText("Mock Fast");

    await pick(/Mock Smart/, "mock-smart");
    await expect(trigger).toContainText("Mock Smart");

    await pick(/Mock Slow/, "mock-slow");
    await expect(trigger).toContainText("Mock Slow");

    // Back to the model the session started on.
    await pick(/Mock Fast/, "mock-fast");
    await expect(trigger).toContainText("Mock Fast");

    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const metadata = sessions.find((item) => item.id === sessionId)?.metadata;
        const overrides = metadata?.runtime_config_overrides as { model?: string } | undefined;
        return overrides?.model;
      })
      .toBe("mock-fast");
  });

  test("applies a switch made right after a page reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ws = watchWs(testPage);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Reloaded Model Switch Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const sessionId = task.session_id;
    if (!sessionId) throw new Error("expected an auto-started session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const metadata = sessions.find((item) => item.id === sessionId)?.metadata;
        return (metadata?.runtime_config as { model?: string } | undefined)?.model;
      })
      .toBe("mock-fast");
    const { trigger, pick } = modelPicker(testPage, ws, sessionId);
    await expect(trigger).toContainText("Mock Fast");

    await pick(/Mock Smart/, "mock-smart");
    await expect(trigger).toContainText("Mock Smart");

    // A reload drops the client-side hydration bookkeeping while the persisted
    // session metadata still names the pre-switch model, so the next switch
    // must not be reverted by that stale copy. watchWs survives page.reload().
    await testPage.reload();
    await session.waitForLoad();
    await expect(trigger).toContainText("Mock Smart");

    await pick(/Mock Fast/, "mock-fast");
    await expect(trigger).toContainText("Mock Fast");
    await expect(trigger).not.toContainText("Mock Smart");
  });
});
