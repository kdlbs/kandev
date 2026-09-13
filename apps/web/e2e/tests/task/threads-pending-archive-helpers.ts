import { expect, type Page, type Route, type TestInfo } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { seedActionThreads, ThreadActionsPage } from "./threads-task-actions-helpers";
import { holdMutationResponse } from "./removal-transition-helpers";
import { swipeDeckLeft } from "./mobile-threads-swipe-helpers";

function watchColumnDeparture(page: Page, taskId: string) {
  return page.evaluateHandle((id) => {
    const selector = `[data-thread-column-id="${id}"]`;
    const containsColumn = (node: Node) =>
      node instanceof Element && (node.matches(selector) || node.querySelector(selector));
    const state = { departed: false, reappeared: false, disconnect: () => observer.disconnect() };
    const observer = new MutationObserver((records) => {
      for (const record of records) {
        if ([...record.removedNodes].some(containsColumn)) state.departed = true;
        if (state.departed && [...record.addedNodes].some(containsColumn)) state.reappeared = true;
      }
    });
    observer.observe(document.body, { subtree: true, childList: true });
    return state;
  }, taskId);
}

// @covers AC-TASKS-THREADS-ACTIONS-003.4, AC-TASKS-THREADS-ACTIONS-003.7, AC-TASKS-THREADS-ACTIONS-003.8, AC-TASKS-THREADS-ACTIONS-004.6
export async function pendingArchiveRecovery(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
  testInfo: TestInfo,
) {
  const { a, b } = await seedActionThreads(api, seed);
  const ui = new ThreadActionsPage(page, mobile);
  await api.saveUserSettings({ confirm_task_archive: true });
  const path = `**/api/v1/tasks/${a.id}/archive`;
  let pending: Route | null = null;
  const handler = (route: Route) => {
    pending = route;
  };
  await page.route(path, handler);
  try {
    await page.goto(`/threads?workspace=${seed.workspaceId}&taskId=${a.id}`);
    await ui.open(a.id);
    await ui.pick("Archive");
    await expect(page.getByTestId("archive-task-confirm")).toBeVisible();
    await expect(ui.column(a.id)).toHaveCount(1);
    await ui.press(page.getByRole("button", { name: "Cancel", exact: true }));
    await expect(ui.column(a.id)).toHaveCount(1);
    expect(pending).toBeNull();

    await ui.open(a.id);
    await ui.pick("Archive");
    await ui.press(page.getByTestId("archive-task-confirm"));
    await expect.poll(() => pending !== null).toBe(true);
    await expect(ui.column(a.id)).toHaveCount(0);
    await expect(ui.trigger(b.id)).toBeInViewport();
    if (mobile) {
      await expect(page.getByTestId("thread-swipe-cue")).toHaveCount(0);
      await ui.press(ui.column(b.id).getByTestId("thread-picker-trigger"));
      await expect(page.getByTestId(`thread-picker-row-${a.id}`)).toHaveCount(0);
      await expect(page.getByTestId(`thread-picker-row-${b.id}`)).toHaveCount(1);
      await ui.press(page.getByTestId(`thread-picker-row-${b.id}`));
    }
    const editor = ui.column(b.id).locator(".tiptap.ProseMirror");
    await ui.press(editor);
    await editor.fill("Keep reading and drafting while archive finishes");
    await expect(editor).toBeFocused();
    await page.screenshot({ path: testInfo.outputPath("archive-pending.png") });

    await pending!.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "Archive temporarily unavailable" }),
    });
    pending = null;
    await expect(page.getByText("Failed to archive task", { exact: true })).toBeVisible();
    await expect(ui.column(a.id)).toHaveCount(1);
    await expect(ui.column(a.id)).not.toHaveAttribute("data-focused", "true");
    await expect(editor).toHaveText("Keep reading and drafting while archive finishes");
    await expect(editor).toBeFocused();
    await expect(page).toHaveURL(
      new RegExp(`/threads\\?workspace=${seed.workspaceId}&taskId=${a.id}$`),
    );
    if (mobile) {
      await expect(page.getByTestId("thread-swipe-cue")).toHaveText("1/2");
      await swipeDeckLeft(page, async () => {
        await expect(page.getByTestId("thread-swipe-cue")).toHaveText("2/2");
      });
      await expect(ui.trigger(a.id)).toBeInViewport();
    }
    await page.screenshot({ path: testInfo.outputPath("archive-rejected.png") });
  } finally {
    if (pending) await (pending as Route).abort();
    await page.unroute(path, handler);
  }
}

// @covers AC-TASKS-THREADS-ACTIONS-003.3, AC-TASKS-THREADS-ACTIONS-003.7
export async function pendingLastArchive(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
  testInfo: TestInfo,
) {
  const { a, b } = await seedActionThreads(api, seed);
  expect((await api.rawRequest("POST", `/api/v1/tasks/${b.id}/archive`, {})).ok).toBe(true);
  await api.saveUserSettings({ confirm_task_archive: false });
  const ui = new ThreadActionsPage(page, mobile);
  await page.goto(`/threads?workspace=${seed.workspaceId}&taskId=${a.id}`);
  await expect(ui.column(a.id).locator(".tiptap.ProseMirror")).toBeVisible();
  if (mobile) await expect(page.getByTestId("threads-mobile-topbar")).toBeInViewport();
  const held = await holdMutationResponse(page, `**/api/v1/tasks/${a.id}/archive`, "POST");
  const departure = await watchColumnDeparture(page, a.id);
  try {
    await ui.open(a.id);
    await ui.pick("Archive");
    const empty = page.getByTestId("threads-empty-state");
    await expect(empty).toBeVisible();
    await held.backendResponseReady;
    expect(held.requestCount()).toBe(1);
    await expect(ui.column(a.id)).toHaveCount(0);
    await expect(page.getByTestId("thread-swipe-cue")).toHaveCount(0);
    await expect(page.getByText("Archived 1 task.", { exact: true })).toHaveCount(0);
    if (mobile) await expect(page.getByTestId("threads-mobile-topbar")).toBeInViewport();
    await page.screenshot({ path: testInfo.outputPath("last-archive-pending.png") });
    held.release();
    await expect(page.getByText("Archived 1 task.", { exact: true })).toBeVisible();
    await expect(empty).toBeVisible();
    await expect(empty).toBeFocused();
    await expect(ui.column(a.id)).toHaveCount(0);
    expect(
      await departure.evaluate(({ departed, reappeared }) => ({ departed, reappeared })),
    ).toEqual({ departed: true, reappeared: false });
    await departure.evaluate((state) => state.disconnect());
    await page.reload();
    await expect(empty).toBeVisible();
  } finally {
    held.release();
    await held.dispose();
    await departure.evaluate((state) => state.disconnect()).catch(() => undefined);
    await departure.dispose();
  }
}
