import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

test.describe("mobile: session deletion", () => {
  test("reveals terminal helper sessions through the mobile history control", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile session history", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const primary = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
      sessionId: `mobile-history-primary-${task.id}`,
    });
    const helper = await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      agentProfileId: seedData.agentProfileId,
      sessionId: `mobile-history-helper-${task.id}`,
      completedAt: "2026-09-01T00:00:00Z",
    });
    await apiClient.setPrimarySession(primary.session_id);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const layout = testPage.locator("[data-testid='mobile-task-layout']:visible");
    await layout.getByTestId("mobile-sessions-pill").tap();
    const sheet = testPage.getByRole("dialog", { name: "Sessions" });
    const helperRow = sheet.getByTestId(`mobile-session-row-${helper.session_id}`);
    await expect(helperRow).toHaveCount(0);
    await prCapture.screenshot("session-history-hidden-mobile", {
      caption: "Mobile session sheet with terminal helper sessions hidden by default",
    });

    const historyToggle = sheet.getByTestId("mobile-session-history-toggle");
    await expect(historyToggle).toBeVisible();
    await historyToggle.tap();
    await expect(helperRow).toBeVisible();
    await prCapture.screenshot("session-history-visible-mobile", {
      caption: "Mobile session sheet after the history control reveals the terminal helper",
    });
  });

  test("deletes a session from the native session actions sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile session deletion",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for the primary session to finish" },
      )
      .toBe(true);

    const { sessions: initialSessions } = await apiClient.listTaskSessions(task.id);
    const primarySessionId = initialSessions[0]?.id;
    if (!primarySessionId) throw new Error("task did not create a primary session");

    const secondarySession = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
      sessionId: `mobile-delete-${task.id}`,
      startedAt: "2026-01-01T00:01:00Z",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const layout = testPage.locator("[data-testid='mobile-task-layout']:visible");
    const pill = layout.getByTestId("mobile-sessions-pill");
    await expect(pill).toBeVisible({ timeout: 30_000 });
    await pill.tap();

    const sheet = testPage.getByRole("dialog", { name: "Sessions" });
    await expect(sheet).toBeVisible({ timeout: 5_000 });
    const secondaryRow = sheet.getByTestId(`mobile-session-row-${secondarySession.session_id}`);
    await expect(secondaryRow).toBeVisible();

    // Radix's dropdown trigger is a mouse/click surface even inside the
    // touch-sized mobile sheet; the surrounding picker and row remain touch-tested.
    await secondaryRow.getByRole("button", { name: "Session actions" }).click();
    const actionsMenu = testPage.getByRole("menu");
    await expect(actionsMenu).toBeVisible({ timeout: 5_000 });
    await actionsMenu.getByRole("menuitem", { name: "Delete" }).tap();

    const confirmation = secondaryRow.getByTestId("mobile-session-delete-confirmation");
    await expect(confirmation).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    // The row-local confirmation reflects archive-or-delete behavior and
    // explicitly says the task workspace and files are retained.
    await expect(confirmation).toContainText("removes the session from active views");
    await expect(confirmation).toContainText("task workspace and its files are kept");

    await confirmation.getByRole("button", { name: "Cancel" }).tap();
    await expect(confirmation).not.toBeVisible();
    await expect(secondaryRow).toBeVisible();

    await secondaryRow.getByRole("button", { name: "Session actions" }).click();
    const reopenedActionsMenu = testPage.getByRole("menu");
    await expect(reopenedActionsMenu).toBeVisible({ timeout: 5_000 });
    await reopenedActionsMenu.getByRole("menuitem", { name: "Delete" }).tap();
    await secondaryRow.getByTestId("mobile-session-delete-confirm").tap();

    await expect(secondaryRow).not.toBeVisible({ timeout: 15_000 });
    await expect(sheet.getByTestId(`mobile-session-row-${primarySessionId}`)).toBeVisible();

    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions.map((item) => item.id)).toEqual([primarySessionId]);
  });
});
