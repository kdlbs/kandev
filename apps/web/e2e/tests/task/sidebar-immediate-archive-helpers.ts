import { expect, type Page, type Route } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { SessionPage } from "../../pages/session-page";

// @covers AC-TASKS-REMOVAL-NAVIGATION-003.1, AC-TASKS-REMOVAL-NAVIGATION-003.2, AC-TASKS-REMOVAL-NAVIGATION-003.3, AC-TASKS-REMOVAL-NAVIGATION-004.1, AC-TASKS-REMOVAL-NAVIGATION-004.2, AC-TASKS-REMOVAL-NAVIGATION-004.3
export async function checkImmediateArchive(options: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  mobile: boolean;
  prCapture: PrAssetCapture;
}) {
  const { page, api, seed, mobile } = options;
  const { settings } = await api.getUserSettings();
  const baseline =
    typeof settings.confirm_task_archive === "boolean" ? settings.confirm_task_archive : undefined;
  await api.saveUserSettings({ confirm_task_archive: true });
  const taskOptions = { workflow_id: seed.workflowId, workflow_step_id: seed.startStepId };
  const nav = await api.seedTask(seed.workspaceId, "Keep selected", taskOptions);
  const target = await api.seedTask(seed.workspaceId, "Archive immediately", taskOptions);
  const path = `**/api/v1/tasks/${target.task_id}/archive`;
  let pending: Route | null = null;
  const handler = (route: Route) => {
    pending = route;
  };
  await page.route(path, handler);
  const session = new SessionPage(page);
  const sheet = page.getByRole("dialog", { name: "Tasks", exact: true });
  const progressToast = () =>
    page.getByTestId("toast-message").filter({ hasText: "Archiving in progress" });
  const rows = () => (mobile ? sheet : session.sidebar).getByTestId("sidebar-task-item");
  const targetRow = () => rows().filter({ hasText: "Archive immediately" });
  const press = async (locator: ReturnType<Page["getByTestId"]>) => {
    if (mobile) await locator.tap();
    else await locator.click();
  };
  const openPicker = async () => {
    if (mobile && !(await sheet.isVisible())) await page.getByTestId("mobile-session-menu").tap();
  };
  const openArchive = async () => {
    await openPicker();
    if (mobile) {
      await targetRow().getByRole("button", { name: "Task actions" }).tap();
      await page.getByRole("menuitem", { name: "Archive", exact: true }).tap();
      await expect(sheet).toBeHidden();
    } else {
      await session.openSidebarMenuAndClick("Archive immediately", "Archive");
    }
  };
  try {
    await page.goto(`/t/${nav.task_id}`);
    await session.waitForLoad();
    await openArchive();
    await press(page.getByRole("button", { name: "Cancel", exact: true }));
    await openPicker();
    await expect(targetRow()).toBeVisible();
    expect(pending).toBeNull();
    await expect(progressToast()).toHaveCount(0);

    await openArchive();
    await press(page.getByTestId("archive-task-confirm"));
    await expect.poll(() => pending !== null).toBe(true);
    await expect(progressToast()).toBeVisible();
    await expect(progressToast().locator("svg")).toHaveClass(/animate-spin/);
    await expect(page.getByTestId("toast-container")).toHaveAttribute("aria-live", "polite");
    await expect(progressToast()).toBeInViewport();
    await expect
      .poll(async () => {
        const currentViewport = page.viewportSize();
        const currentBox = await progressToast().boundingBox();
        if (!currentViewport || !currentBox) return null;
        return Math.round(currentViewport.width - currentBox.x - currentBox.width);
      })
      .toBe(16);
    const viewport = page.viewportSize();
    const toastBox = await progressToast().boundingBox();
    expect(viewport).not.toBeNull();
    expect(toastBox).not.toBeNull();
    expect(toastBox!.x).toBeGreaterThanOrEqual(0);
    expect(viewport!.width - toastBox!.x - toastBox!.width).toBeGreaterThanOrEqual(0);
    expect(viewport!.width - toastBox!.x - toastBox!.width).toBeLessThanOrEqual(24);
    expect(viewport!.height - toastBox!.y - toastBox!.height).toBeGreaterThanOrEqual(0);
    expect(viewport!.height - toastBox!.y - toastBox!.height).toBeLessThanOrEqual(80);
    await options.prCapture.screenshot(
      mobile ? "archive-progress-mobile" : "archive-progress-desktop",
      {
        caption: mobile
          ? "A phone task archive stays visibly in progress while the request is pending."
          : "A desktop task archive stays visibly in progress while the request is pending.",
        fullPage: true,
      },
    );
    if (mobile) await page.getByTestId("mobile-session-menu").tap();
    await expect(targetRow()).toBeVisible();
    await expect(targetRow()).toHaveAttribute("aria-busy", "true");
    await expect(targetRow()).toHaveClass(/opacity-60/);
    await expect(targetRow().getByTestId("task-state-archive-pending")).toBeVisible();
    await expect(rows().filter({ hasText: "Keep selected" })).toBeInViewport();
    await expect(page).toHaveURL(new RegExp(`/t/${nav.task_id}$`));
    await expect(progressToast()).toBeVisible();
    await expect(rows().filter({ hasText: "Keep selected" })).toBeInViewport();
    await pending!.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "Archive unavailable" }),
    });
    pending = null;
    await expect(progressToast()).toHaveCount(0);
    if (mobile) {
      await openPicker();
    } else {
      await expect(page.getByText("Failed to archive task", { exact: true })).toBeVisible();
    }
    await expect(targetRow()).toBeVisible();
    await expect(targetRow()).not.toHaveAttribute("aria-busy");
    await expect(targetRow().getByTestId("task-state-archive-pending")).toHaveCount(0);
    await expect(page).toHaveURL(new RegExp(`/t/${nav.task_id}$`));

    await openArchive();
    await press(page.getByTestId("archive-task-confirm"));
    await expect.poll(() => pending !== null).toBe(true);
    await expect(progressToast()).toBeVisible();
    if (mobile) await page.getByTestId("mobile-session-menu").tap();
    await expect(targetRow()).toBeVisible();
    await expect(targetRow()).toHaveAttribute("aria-busy", "true");
    await expect(targetRow().getByTestId("task-state-archive-pending")).toBeVisible();
    await pending!.continue();
    pending = null;
    await expect(progressToast()).toHaveCount(0);
    await expect(
      page.getByTestId("toast-message").filter({ hasText: "Archived 1 task." }),
    ).toBeVisible();
    await expect(targetRow()).toHaveCount(0);
  } finally {
    if (pending) await (pending as Route).abort();
    await page.unroute(path, handler);
    await api.saveUserSettings({ confirm_task_archive: baseline });
  }
}
