import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { openQuickChatWithAgent, sendQuickChatMessage } from "./quick-chat-helpers";

const SETTLED_STATES = new Set(["IDLE", "WAITING_FOR_INPUT", "COMPLETED", "FAILED", "CANCELLED"]);

function waitForSessionSettled(ws: ReturnType<typeof watchWs>, sessionId: string) {
  return ws.waitForEvent("session.state_changed", {
    where: (payload) =>
      payload.session_id === sessionId && SETTLED_STATES.has(payload.new_state ?? ""),
  });
}

test.describe("quick chat activity indicators", () => {
  test("shows tab and sidebar running state, then clears a finished state when opened", async ({
    testPage,
  }) => {
    const ws = watchWs(testPage);
    const created = testPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    const dialog = await openQuickChatWithAgent(testPage);
    const { session_id: sessionId } = (await (await created).json()) as { session_id: string };
    const tab = dialog.getByTestId("quick-chat-tab");
    const shortcut = testPage.getByTestId("sidebar-quick-chat-shortcut");
    const indicator = shortcut.getByTestId("quick-chat-activity-indicator");

    await expect(tab).toHaveCount(1);
    const completed = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    await sendQuickChatMessage(dialog, testPage, "/slow 8s");
    await expect(tab.getByRole("status")).toBeVisible();
    const settled = waitForSessionSettled(ws, sessionId);

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
  }) => {
    const ws = watchWs(tabletTestPage);
    const created = tabletTestPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    const dialog = await openQuickChatWithAgent(tabletTestPage);
    const { session_id: sessionId } = (await (await created).json()) as { session_id: string };
    const tab = dialog.getByTestId("quick-chat-tab");
    const button = tabletTestPage.getByTestId("tablet-quick-chat-button");
    const indicator = button.getByTestId("quick-chat-activity-indicator");

    await expect(tab).toHaveCount(1);
    const completed = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    await sendQuickChatMessage(dialog, tabletTestPage, "/slow 8s");
    await expect(tab.getByRole("status")).toBeVisible();
    const settled = waitForSessionSettled(ws, sessionId);

    await tabletTestPage.keyboard.press("Escape");
    await expect(indicator).toHaveAttribute("data-state", "running");

    await Promise.all([completed, settled]);
    await expect(indicator).toHaveAttribute("data-state", "finished");

    await button.click();
    await expect(indicator).toHaveCount(0);
  });
});
