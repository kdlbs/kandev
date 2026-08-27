import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import {
  openQuickChatWithAgent,
  sendQuickChatMessage,
  waitForSessionSettled,
  waitForSessionSettledBaseline,
} from "./quick-chat-helpers";

test.describe("quick chat activity indicators", () => {
  test("shows tab and sidebar running state, then clears a finished state when opened", async ({
    testPage,
    apiClient,
  }) => {
    const ws = watchWs(testPage);
    const created = testPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    const dialog = await openQuickChatWithAgent(testPage);
    const { session_id: sessionId, task_id: taskId } = (await (await created).json()) as {
      session_id: string;
      task_id: string;
    };
    const tab = dialog.getByTestId("quick-chat-tab");
    const shortcut = testPage.getByTestId("sidebar-quick-chat-shortcut");
    const indicator = shortcut.getByTestId("quick-chat-activity-indicator");

    await expect(tab).toHaveCount(1);
    await waitForSessionSettledBaseline(apiClient, taskId, sessionId);
    const completed = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    const settled = waitForSessionSettled(ws, sessionId);
    await sendQuickChatMessage(dialog, testPage, "/slow 8s");
    await expect(tab.getByRole("status")).toBeVisible();

    await testPage.keyboard.press("Escape");
    await expect(indicator).toHaveAttribute("data-state", "running");

    await Promise.all([completed, settled]);
    await expect(indicator).toHaveAttribute("data-state", "finished");

    await shortcut.click();
    await expect(indicator).toHaveCount(0);
    await expect(testPage.getByRole("dialog", { name: "Quick Chat" })).toBeVisible();

    await testPage.keyboard.press("Escape");
    await expect(indicator).toHaveCount(0);
  });

  test("uses the same running-to-finished state sequence in the tablet header", async ({
    tabletTestPage,
    apiClient,
  }) => {
    const ws = watchWs(tabletTestPage);
    const created = tabletTestPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    const dialog = await openQuickChatWithAgent(tabletTestPage);
    const { session_id: sessionId, task_id: taskId } = (await (await created).json()) as {
      session_id: string;
      task_id: string;
    };
    const tab = dialog.getByTestId("quick-chat-tab");
    const button = tabletTestPage.getByTestId("tablet-quick-chat-button");
    const indicator = button.getByTestId("quick-chat-activity-indicator");

    await expect(tab).toHaveCount(1);
    await waitForSessionSettledBaseline(apiClient, taskId, sessionId);
    const completed = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    const settled = waitForSessionSettled(ws, sessionId);
    await sendQuickChatMessage(dialog, tabletTestPage, "/slow 8s");
    await expect(tab.getByRole("status")).toBeVisible();

    await tabletTestPage.keyboard.press("Escape");
    await expect(indicator).toHaveAttribute("data-state", "running");

    await Promise.all([completed, settled]);
    await expect(indicator).toHaveAttribute("data-state", "finished");

    await button.click();
    await expect(indicator).toHaveCount(0);
  });
});
