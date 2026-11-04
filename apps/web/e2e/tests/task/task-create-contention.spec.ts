import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { seedRunningGeneratingSession } from "../../helpers/generating-session";
import { SessionPage } from "../../pages/session-page";
import { SidebarTasksPage } from "../../pages/sidebar-tasks-page";
import {
  expectStandardCreateReady,
  fillRecoveryDraft,
  openDesktopCreateDialog,
} from "./task-create-recovery-helpers";

useRegularMode();

function sessionState(
  sessions: { sessions: Array<{ id: string; state: string }> },
  sessionId: string,
) {
  return sessions.sessions.find((session) => session.id === sessionId)?.state;
}

test.describe("Task creation under concurrent activity", () => {
  test("creates while another task streams and keeps the other task's saved color", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(180_000);
    const active = await seedRunningGeneratingSession(
      testPage,
      apiClient,
      seedData,
      `Contention active task ${testInfo.workerIndex}-${Date.now()}`,
    );

    const title = `Contention desktop ${testInfo.workerIndex}-${testInfo.repeatEachIndex}-${Date.now()}`;
    const dialog = await openDesktopCreateDialog(testPage);
    await fillRecoveryDraft(dialog, title, "Create while another task is generating.");

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
    await expect
      .poll(() => new URL(testPage.url()).pathname, { timeout: 15_000 })
      .toBe(`/t/${created.id}`);
    await new SessionPage(testPage).waitForLoad();

    const sidebar = new SidebarTasksPage(testPage);
    const activeRow = sidebar.row(active.taskId);
    await expect(activeRow).toBeVisible();

    await sidebar.rightClick(active.taskId);
    const colorMenu = testPage.getByRole("menuitem", { name: /color/i });
    await expect(colorMenu).toBeVisible();
    await colorMenu.hover();
    await testPage.getByRole("menuitemradio", { name: "Blue", exact: true }).click();

    const marker = activeRow.getByTestId("task-item-color-marker");
    await expect(marker).toHaveAttribute("data-color-token", "blue");
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        const colors = settings.sidebar_task_colors as Record<string, string | null> | undefined;
        return colors?.[active.taskId];
      })
      .toBe("blue");
    await expect
      .poll(async () =>
        sessionState(await apiClient.listTaskSessions(active.taskId), active.sessionId),
      )
      .toBe("RUNNING");

    await testPage.reload();
    await new SessionPage(testPage).waitForLoad();
    await expect(sidebar.row(active.taskId).getByTestId("task-item-color-marker")).toHaveAttribute(
      "data-color-token",
      "blue",
    );
    await expect
      .poll(async () =>
        sessionState(await apiClient.listTaskSessions(active.taskId), active.sessionId),
      )
      .toBe("RUNNING");
    await expect(testPage).toHaveURL(new RegExp(`/t/${created.id}$`));
  });
});
