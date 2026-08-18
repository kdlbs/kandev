import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import { sendQuickChatMessage, startQuickChatFromSetup } from "./quick-chat-helpers";

test.describe("quick chat idle dot", () => {
  test("marks the mobile header after a closed quick chat turn completes", async ({ testPage }) => {
    const ws = watchWs(testPage);
    await testPage.goto("/");
    await testPage.getByTestId("mobile-quick-chat-button").tap();
    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    const created = testPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    await startQuickChatFromSetup(dialog, testPage);
    await expect(
      testPage.getByRole("status", { name: /Agent is (starting|running)/ }),
    ).not.toBeVisible();
    const { session_id: sessionId } = (await (await created).json()) as { session_id: string };
    const button = testPage.getByTestId("mobile-quick-chat-button");
    const completed = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    await sendQuickChatMessage(dialog, testPage, "/slow 8s");
    await expect(dialog.getByText("Running slow response", { exact: false })).toBeVisible();
    await dialog.getByTestId("quick-chat-close").tap();
    await completed;
    await expect(button.getByTestId("quick-chat-unseen-dot")).toBeVisible({ timeout: 15_000 });

    await button.tap();
    await expect(button.getByTestId("quick-chat-unseen-dot")).toHaveCount(0);
    // The reopened dialog re-subscribes over WS; wait until the previous
    // exchange has rendered so the send cannot race a dead subscription.
    await expect(dialog.getByText("Slow response complete", { exact: false })).toBeVisible({
      timeout: 15_000,
    });
    const secondCompleted = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    await sendQuickChatMessage(dialog, testPage, "/slow 8s");
    await dialog.getByTestId("quick-chat-close").tap();
    await secondCompleted;
    await expect(button.getByTestId("quick-chat-unseen-dot")).toBeVisible({ timeout: 15_000 });
  });

  test("marks the task switcher entry after a closed quick chat turn completes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ws = watchWs(testPage);
    const seeded = await apiClient.seedTask(seedData.workspaceId, "Mobile Quick Chat Idle Dot", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${seeded.task_id}`);
    await new SessionPage(testPage).waitForLoad();

    await testPage.getByTestId("mobile-session-menu").tap();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const entry = sheet.getByTestId("mobile-sheet-quick-chat");
    await expect(entry.getByTestId("quick-chat-unseen-dot")).toHaveCount(0);

    const created = testPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    await entry.tap();
    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await startQuickChatFromSetup(dialog, testPage);
    const { session_id: sessionId } = (await (await created).json()) as { session_id: string };
    const completed = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    await sendQuickChatMessage(dialog, testPage, "/slow 8s");
    await dialog.getByTestId("quick-chat-close").tap();

    await completed;
    await testPage.getByTestId("mobile-session-menu").tap();
    await expect(entry.getByTestId("quick-chat-unseen-dot")).toBeVisible({ timeout: 15_000 });
  });
});
