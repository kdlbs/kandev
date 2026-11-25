import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { seedRunningGeneratingSession } from "../../helpers/generating-session";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";
import {
  expectStandardCreateReady,
  fillRecoveryDraft,
  openMobileCreateDialog,
} from "./task-create-recovery-helpers";

useRegularMode();

function sessionState(
  sessions: { sessions: Array<{ id: string; state: string }> },
  sessionId: string,
) {
  return sessions.sessions.find((session) => session.id === sessionId)?.state;
}

test("phone creates while another task streams and keeps the other task's saved color", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(180_000);
  const active = await seedRunningGeneratingSession(
    testPage,
    apiClient,
    seedData,
    `Contention phone active ${testInfo.workerIndex}-${Date.now()}`,
  );

  const title = `Contention phone ${testInfo.workerIndex}-${testInfo.repeatEachIndex}-${Date.now()}`;
  const dialog = await openMobileCreateDialog(testPage);
  await fillRecoveryDraft(dialog, title, "Create this phone task while another is generating.");

  const createResponse = testPage.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname === "/api/v1/tasks",
  );
  await expectStandardCreateReady(dialog);
  await dialog.getByTestId("submit-start-agent").click();
  const response = await createResponse;
  expect(response.ok()).toBeTruthy();
  const created = (await response.json()) as { id: string; session_id?: string };
  expect(created.id).toBeTruthy();

  await expect(dialog).toBeHidden();
  const kanban = new MobileKanbanPage(testPage);
  const createdCard = kanban.taskCard(created.id);
  await expect(createdCard).toBeVisible();
  await createdCard.click();
  await expect(testPage).toHaveURL(new RegExp(`/t/${created.id}$`));

  const openDrawer = async () => {
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.mobileSessionMenu.tap();
    const drawer = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(drawer).toBeVisible();
    return drawer;
  };

  const drawer = await openDrawer();
  const activeRow = drawer.locator(
    `[data-testid="sidebar-task-item"][data-task-row-id="${active.taskId}"]`,
  );
  await expect(activeRow).toBeVisible();
  await activeRow.click({ button: "right" });
  const colorMenu = testPage.getByRole("menuitem", { name: /color/i });
  await expect(colorMenu).toBeVisible();
  await colorMenu.tap();
  await testPage.getByRole("menuitemradio", { name: "Purple", exact: true }).tap();

  const marker = activeRow.getByTestId("task-item-color-marker");
  await expect(marker).toHaveAttribute("data-color-token", "purple");
  await expect
    .poll(async () => {
      const { settings } = await apiClient.getUserSettings();
      const colors = settings.sidebar_task_colors as Record<string, string | null> | undefined;
      return colors?.[active.taskId];
    })
    .toBe("purple");
  await expect
    .poll(async () =>
      sessionState(await apiClient.listTaskSessions(active.taskId), active.sessionId),
    )
    .toBe("RUNNING");

  await testPage.reload();
  const reloadedDrawer = await openDrawer();
  await expect(
    reloadedDrawer
      .locator(`[data-testid="sidebar-task-item"][data-task-row-id="${active.taskId}"]`)
      .getByTestId("task-item-color-marker"),
  ).toHaveAttribute("data-color-token", "purple");
  await expect
    .poll(async () =>
      sessionState(await apiClient.listTaskSessions(active.taskId), active.sessionId),
    )
    .toBe("RUNNING");
});
