import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { routeMainWebSocketWithDelayedActionResponse } from "../../helpers/ws-drop";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

test.describe("mobile: session deletion", () => {
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

    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Delete" }).tap();

    await expect(secondaryRow).not.toBeVisible({ timeout: 15_000 });
    await expect(sheet.getByTestId(`mobile-session-row-${primarySessionId}`)).toBeVisible();

    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions.map((item) => item.id)).toEqual([primarySessionId]);
  });

  test("disables the confirm control on the mobile sessions sheet until the warning fetch resolves", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile session deletion gating",
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
      sessionId: `mobile-delete-gating-${task.id}`,
      startedAt: "2026-01-01T00:01:00Z",
    });

    // Must attach before navigation: routeWebSocket only affects connections
    // opened after it is registered.
    const delayed = await routeMainWebSocketWithDelayedActionResponse(
      testPage,
      "session.git.snapshots",
    );
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

    await secondaryRow.getByRole("button", { name: "Session actions" }).click();
    const actionsMenu = testPage.getByRole("menu");
    await expect(actionsMenu).toBeVisible({ timeout: 5_000 });
    await actionsMenu.getByRole("menuitem", { name: "Delete" }).tap();

    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();

    // The dialog issues the fetch immediately on open, but the response is
    // held — this is the real WS round trip, not a mocked hook return value.
    const confirmButton = dialog.getByRole("button", { name: "Delete" });
    await expect(confirmButton).toBeDisabled();

    delayed.release();

    await expect(confirmButton).toBeEnabled({ timeout: 10_000 });
    await confirmButton.tap();

    await expect(secondaryRow).not.toBeVisible({ timeout: 15_000 });
    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions.map((item) => item.id)).toEqual([primarySessionId]);
  });
});
