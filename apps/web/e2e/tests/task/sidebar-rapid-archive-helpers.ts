import { expect, type Locator, type Page, type Route } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const ARCHIVE_ROUTE = "**/api/v1/tasks/*/archive";
const CONFIRM_TIMEOUT = 15_000;

type ArchiveSurface = {
  page: Page;
  session: SessionPage;
  sheet: Locator;
  mobile: boolean;
  rowFor: (title: string) => Locator;
  press: (locator: Locator) => Promise<void>;
  openPicker: () => Promise<void>;
  archiveRow: (title: string) => Promise<void>;
};

function taskIdFromArchiveRequest(url: string): string {
  return new URL(url).pathname.split("/").at(-2) ?? "";
}

function taskUrlPattern(taskId: string): RegExp {
  return new RegExp(`/t/${taskId}(?:[?#]|$)`);
}

function archiveSurface(page: Page, mobile: boolean): ArchiveSurface {
  const session = new SessionPage(page);
  const sheet = page.getByRole("dialog", { name: "Tasks", exact: true });
  const rows = () => (mobile ? sheet : session.sidebar).getByTestId("sidebar-task-item");
  const rowFor = (title: string) => rows().filter({ hasText: title });
  const press = async (locator: Locator) => {
    if (mobile) await locator.tap();
    else await locator.click();
  };
  const openPicker = async () => {
    if (mobile && !(await sheet.isVisible())) {
      await page.getByTestId("mobile-task-picker-trigger").tap();
    }
  };
  const archiveRow = async (title: string) => {
    await openPicker();
    if (mobile) {
      await rowFor(title).getByRole("button", { name: "Task actions" }).tap();
      await page.getByRole("menuitem", { name: "Archive" }).tap();
      await expect(sheet).toBeHidden();
    } else {
      await session.openSidebarMenuAndClick(title, "Archive");
    }
    await press(page.getByTestId("archive-task-confirm"));
    await expect(page.getByTestId("archive-task-confirm")).toBeHidden({
      timeout: CONFIRM_TIMEOUT,
    });
  };
  return { page, session, sheet, mobile, rowFor, press, openPicker, archiveRow };
}

async function openArchiveMenuItem(surface: ArchiveSurface, title: string) {
  await surface.openPicker();
  const row = surface.rowFor(title);
  if (surface.mobile) {
    await row.getByRole("button", { name: "Task actions" }).tap();
  } else {
    await row.scrollIntoViewIfNeeded();
    await row.hover();
    await row.getByRole("button", { name: "Task actions" }).click();
  }
  const archiveItem = surface.page.getByRole("menuitem", { name: "Archive" });
  await expect(archiveItem).toBeVisible();
  return archiveItem;
}

function taskOptions(seed: SeedData) {
  return { workflow_id: seed.workflowId, workflow_step_id: seed.startStepId };
}

async function captureRapidArchives(
  surface: ArchiveSurface,
  firstTargetTitle: string,
  mobile: boolean,
  prCapture: PrAssetCapture | undefined,
) {
  if (!prCapture?.capturing) return;
  if (mobile) await surface.rowFor(firstTargetTitle).scrollIntoViewIfNeeded();
  await prCapture.screenshot(mobile ? "rapid-archives-mobile" : "rapid-archives-desktop", {
    caption: mobile
      ? "A phone picker keeps independently accepted archives busy at the same time."
      : "A desktop sidebar keeps independently accepted archives busy at the same time.",
  });
}

async function releaseArchive(route: Route, outcome: "succeed" | "fail") {
  if (outcome === "succeed") {
    await route.continue();
    return;
  }
  await route.fulfill({
    status: 503,
    contentType: "application/json",
    body: JSON.stringify({ error: "Archive unavailable" }),
  });
}

/**
 * Archives several rows without letting any request reach the backend, proves
 * each target keeps its own busy controls, then releases mixed results out of
 * order. Every wait is causal: a held route, a settled row, or the navigation
 * it produces.
 */
// @covers AC-TASKS-REMOVAL-NAVIGATION-002.6, AC-TASKS-REMOVAL-NAVIGATION-003.2
export async function checkRapidIndependentArchives(options: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  mobile: boolean;
  prCapture?: PrAssetCapture;
}) {
  const { page, api, seed, mobile, prCapture } = options;
  const { settings } = await api.getUserSettings();
  const baseline =
    typeof settings.confirm_task_archive === "boolean" ? settings.confirm_task_archive : undefined;
  await api.saveUserSettings({ confirm_task_archive: true });
  const titles = ["Rapid one", "Rapid two", "Rapid three", "Rapid four", "Rapid five"];
  const keep = await api.seedTask(seed.workspaceId, "Keep selected", taskOptions(seed));
  await api.seedTaskSession(keep.task_id, { state: "WAITING_FOR_INPUT" });
  const navigation = await api.seedTask(seed.workspaceId, "Navigation target", taskOptions(seed));
  await api.seedTaskSession(navigation.task_id, { state: "WAITING_FOR_INPUT" });
  const targets: Array<{ title: string; taskId: string }> = [];
  for (const title of titles) {
    const seeded = await api.seedTask(seed.workspaceId, title, taskOptions(seed));
    targets.push({ title, taskId: seeded.task_id });
  }

  const held = new Map<string, Route>();
  const handler = (route: Route) => {
    held.set(taskIdFromArchiveRequest(route.request().url()), route);
  };
  await page.route(ARCHIVE_ROUTE, handler);
  const surface = archiveSurface(page, mobile);
  const progressToast = () =>
    page.getByTestId("toast-message").filter({ hasText: "Archiving in progress" });
  try {
    await page.goto(`/t/${navigation.task_id}`);
    await surface.session.waitForLoad();
    for (const target of targets) {
      await surface.archiveRow(target.title);
      await expect.poll(() => held.has(target.taskId)).toBe(true);
    }

    await expect(progressToast()).toBeVisible();
    await surface.openPicker();
    for (const target of targets) {
      await expect(surface.rowFor(target.title)).toHaveAttribute("aria-busy", "true");
      await expect(
        surface.rowFor(target.title).getByTestId("task-state-removal-pending"),
      ).toBeVisible();
    }
    await captureRapidArchives(surface, targets[0].title, mobile, prCapture);

    const archiveItem = await openArchiveMenuItem(surface, "Keep selected");
    await expect(archiveItem).not.toHaveAttribute("data-disabled", "");
    await page.keyboard.press("Escape");
    await expect(archiveItem).toBeHidden();

    const keepRow = surface.rowFor("Keep selected");
    await keepRow.scrollIntoViewIfNeeded();
    await expect(keepRow).toBeInViewport();
    await surface.press(keepRow);
    await expect(page).toHaveURL(taskUrlPattern(keep.task_id));
    if (!mobile) await expect(surface.session.sidebar).toBeVisible();

    const failed = targets[2];
    await releaseArchive(held.get(failed.taskId)!, "fail");
    held.delete(failed.taskId);
    await surface.openPicker();
    await expect(surface.rowFor(failed.title)).not.toHaveAttribute("aria-busy");
    await expect(surface.rowFor(failed.title)).toBeVisible();
    for (const target of targets.filter((item) => item !== failed)) {
      await expect(surface.rowFor(target.title)).toHaveAttribute("aria-busy", "true");
    }

    for (const target of [...targets].reverse()) {
      const route = held.get(target.taskId);
      if (!route) continue;
      await releaseArchive(route, "succeed");
      held.delete(target.taskId);
    }
    for (const target of targets) {
      if (target === failed) {
        await expect(surface.rowFor(target.title)).toBeVisible();
        await expect(surface.rowFor(target.title)).not.toHaveAttribute("aria-busy");
        continue;
      }
      await expect(surface.rowFor(target.title)).toHaveCount(0);
    }
    await expect(progressToast()).toHaveCount(0);
    await expect(page).toHaveURL(taskUrlPattern(keep.task_id));
  } finally {
    for (const route of held.values()) await route.abort().catch(() => undefined);
    await page.unroute(ARCHIVE_ROUTE, handler);
    await api.saveUserSettings({ confirm_task_archive: baseline });
  }
}

/**
 * Holds replacement reads and requests so an independently accepted archive is
 * pending when the selected task looks for its destination. B must never be
 * rendered as that destination; the next eligible candidate wins.
 */
// @covers AC-TASKS-REMOVAL-NAVIGATION-001.6
export async function checkDelayedReplacementArchive(options: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  mobile: boolean;
}) {
  const { page, api, seed, mobile } = options;
  const { settings } = await api.getUserSettings();
  const baseline =
    typeof settings.confirm_task_archive === "boolean" ? settings.confirm_task_archive : undefined;
  await api.saveUserSettings({ confirm_task_archive: true });
  const selected = await api.seedTask(seed.workspaceId, "Archive selected", taskOptions(seed));
  const delayed = await api.seedTask(seed.workspaceId, "Delayed candidate", taskOptions(seed));
  const fallback = await api.seedTask(seed.workspaceId, "Fallback candidate", taskOptions(seed));

  const heldReads: Route[] = [];
  let heldArchive: Route | null = null;
  const readHandler = (route: Route) => {
    heldReads.push(route);
  };
  const archiveHandler = (route: Route) => {
    heldArchive = route;
  };
  await page.route(`**/api/v1/tasks/${delayed.task_id}`, readHandler);
  await page.route(`**/api/v1/tasks/${delayed.task_id}/archive`, archiveHandler);
  const surface = archiveSurface(page, mobile);
  try {
    await page.goto(`/t/${selected.task_id}`);
    await surface.session.waitForLoad();
    if (mobile) {
      // The phone surface replaces the departing view with its status while the
      // replacement loads, so the second acceptance is issued first. It is the
      // same eligibility rule: B is already accepted when A looks for a
      // destination and must never render as that destination.
      await surface.archiveRow("Delayed candidate");
      await expect.poll(() => heldArchive !== null).toBe(true);
      await surface.archiveRow("Archive selected");
    } else {
      await surface.archiveRow("Archive selected");
      await expect.poll(() => heldReads.length > 0).toBe(true);
      await surface.archiveRow("Delayed candidate");
      await expect.poll(() => heldArchive !== null).toBe(true);
      await expect(surface.rowFor("Delayed candidate")).toHaveAttribute("aria-busy", "true");
      await expect(page).toHaveURL(taskUrlPattern(selected.task_id));
      for (const route of heldReads.splice(0)) await route.continue();
    }

    await expect(page).toHaveURL(taskUrlPattern(fallback.task_id));
    await expect(page).not.toHaveURL(taskUrlPattern(delayed.task_id));

    // Forwarding the held request only starts the mutation. Wait for the
    // response and the confirmed backend state, then assert the same
    // viewport-aware row surface, not the desktop sidebar on a phone.
    const archiveSettled = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().includes(`/api/v1/tasks/${delayed.task_id}/archive`),
    );
    await heldArchive!.continue();
    heldArchive = null;
    await archiveSettled;
    await expect.poll(async () => (await api.getTask(delayed.task_id)).archived_at).toBeTruthy();

    await surface.openPicker();
    await expect(surface.rowFor("Fallback candidate")).toBeVisible();
    await expect(surface.rowFor("Delayed candidate")).toHaveCount(0);
    await expect(page).toHaveURL(taskUrlPattern(fallback.task_id));
  } finally {
    for (const route of heldReads.splice(0)) await route.abort().catch(() => undefined);
    if (heldArchive) await (heldArchive as Route).abort().catch(() => undefined);
    await page.unroute(`**/api/v1/tasks/${delayed.task_id}`, readHandler);
    await page.unroute(`**/api/v1/tasks/${delayed.task_id}/archive`, archiveHandler);
    await api.saveUserSettings({ confirm_task_archive: baseline });
  }
}
