import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import {
  sendQuickChatMessage,
  startQuickChatFromSetup,
  waitForSessionSettled,
  waitForSessionSettledBaseline,
} from "./quick-chat-helpers";

test.describe("quick chat activity indicators", () => {
  test("shows the mobile header running and finished states through touch", async ({
    testPage,
    apiClient,
  }) => {
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
    const { session_id: sessionId, task_id: taskId } = (await (await created).json()) as {
      session_id: string;
      task_id: string;
    };
    const tab = dialog.getByTestId("quick-chat-tab");
    const button = testPage.getByTestId("mobile-quick-chat-button");
    const indicator = button.getByTestId("quick-chat-activity-indicator");

    await expect(tab).toHaveCount(1);
    await waitForSessionSettledBaseline(apiClient, taskId, sessionId);
    const completed = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    const settled = waitForSessionSettled(ws, sessionId);
    await sendQuickChatMessage(dialog, testPage, "/slow 8s");
    await expect(tab.getByRole("status")).toBeVisible();

    await dialog.getByTestId("quick-chat-close").tap();
    await expect(indicator).toHaveAttribute("data-state", "running");

    await Promise.all([completed, settled]);
    await expect(indicator).toHaveAttribute("data-state", "finished");

    await button.tap();
    await expect(indicator).toHaveCount(0);
    await expect(dialog).toBeVisible();
  });

  test("keeps the task-switcher entry in sync with the same activity state", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ws = watchWs(testPage);
    const seeded = await apiClient.seedTask(seedData.workspaceId, "Mobile Quick Chat Activity", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${seeded.task_id}`);
    await new SessionPage(testPage).waitForLoad();

    await testPage.getByTestId("mobile-session-menu").tap();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const entry = sheet.getByTestId("mobile-sheet-quick-chat");
    await expect(entry.getByTestId("quick-chat-activity-indicator")).toHaveCount(0);

    const created = testPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    await entry.tap();
    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await startQuickChatFromSetup(dialog, testPage);
    const { session_id: sessionId, task_id: taskId } = (await (await created).json()) as {
      session_id: string;
      task_id: string;
    };
    await waitForSessionSettledBaseline(apiClient, taskId, sessionId);
    const completed = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    const settled = waitForSessionSettled(ws, sessionId);
    await sendQuickChatMessage(dialog, testPage, "/slow 8s");
    await expect(dialog.getByTestId("quick-chat-tab").getByRole("status")).toBeVisible();
    await dialog.getByTestId("quick-chat-close").tap();

    await testPage.getByTestId("mobile-session-menu").tap();
    const openEntry = testPage
      .getByRole("dialog", { name: "Tasks" })
      .getByTestId("mobile-sheet-quick-chat");
    await expect(openEntry.getByTestId("quick-chat-activity-indicator")).toHaveAttribute(
      "data-state",
      "running",
    );
    await testPage.keyboard.press("Escape");

    await Promise.all([completed, settled]);
    await testPage.getByTestId("mobile-session-menu").tap();
    const finishedEntry = testPage
      .getByRole("dialog", { name: "Tasks" })
      .getByTestId("mobile-sheet-quick-chat");
    await expect(finishedEntry.getByTestId("quick-chat-activity-indicator")).toHaveAttribute(
      "data-state",
      "finished",
    );
  });
});
