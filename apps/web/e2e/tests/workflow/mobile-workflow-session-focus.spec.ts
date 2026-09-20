import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createWorkflowSessionFocusScenario,
  waitForNewWorkflowSession,
} from "./workflow-session-focus-helpers";

test.describe("Mobile workflow session focus", () => {
  test("shows the new committed recipient in chat without opening the keyboard", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const scenario = await createWorkflowSessionFocusScenario(apiClient, seedData, "new");
    await testPage.goto(`/t/${scenario.task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const moveResponse = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname === `/api/v1/tasks/${scenario.task.id}/move`,
    );
    await testPage.getByTestId("mobile-session-menu").tap();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = sheet.locator(`[data-task-row-id="${scenario.task.id}"]`);
    await expect(taskRow).toBeVisible();
    await taskRow.getByRole("button", { name: "Task actions" }).tap();
    await testPage.getByTestId("task-context-move-to").tap();
    await testPage.getByTestId(`task-context-step-${scenario.destination.id}`).tap();
    const response = await moveResponse;
    expect(response.ok()).toBe(true);
    expect((await response.json()).workflow_entry_identity).toMatch(/^entry:/);

    const destinationSessionId = await waitForNewWorkflowSession(apiClient, scenario.task.id, [
      scenario.sourceSessionId,
      scenario.secondarySessionId,
    ]);
    await expect(session.activeChat()).toHaveAttribute("data-session-id", destinationSessionId);
    await expect(testPage.getByTestId("mobile-sessions-pill")).toBeVisible();
    await expect(testPage.getByTestId("workflow-step-disclosure")).not.toBeVisible();

    const focusedEditable = await testPage.evaluate(() => {
      const active = document.activeElement;
      return active instanceof HTMLElement && active.matches("input, textarea, [contenteditable]");
    });
    expect(focusedEditable).toBe(false);

    const documentWidth = await testPage.evaluate(() => ({
      clientWidth: document.documentElement.clientWidth,
      scrollWidth: document.documentElement.scrollWidth,
    }));
    expect(documentWidth.scrollWidth).toBeLessThanOrEqual(documentWidth.clientWidth + 1);
  });
});
