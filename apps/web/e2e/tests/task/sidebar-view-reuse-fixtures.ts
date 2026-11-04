import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";

function gate() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { promise, release };
}

function deferredVoid() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

/** Resolves when any single sidebar query reaches its fifth rejected attempt. */
function sidebarFailureBarrier() {
  const counts = new Map<string, number>();
  const exhausted = deferredVoid();
  const count = (body: unknown) => {
    const key = JSON.stringify(body);
    const attempts = (counts.get(key) ?? 0) + 1;
    counts.set(key, attempts);
    if (attempts === 5) exhausted.resolve();
  };
  return { count, exhausted: exhausted.promise };
}

async function retrySidebarRecovery(page: Page, surface: ReturnType<Page["locator"]>) {
  const retry = surface.getByRole("button", { name: "Retry", exact: true });
  await expect(retry).toBeEnabled();
  const recovered = waitForHttp(page, "POST", /\/sidebar\/query$/, {
    predicate: (response) => response.ok(),
  });
  await retry.click();
  expect((await recovered).ok()).toBe(true);
  await expect(surface.getByRole("alert")).toHaveCount(0);
}

function taskRowTop(surface: ReturnType<Page["locator"]>, taskId: string) {
  return surface.evaluate((element, id) => {
    const row = element.querySelector(`[data-task-row-id="${id}"]`)!;
    return row.getBoundingClientRect().top - element.getBoundingClientRect().top;
  }, taskId);
}

async function seedViews(api: ApiClient, seed: SeedData, directories: string[]) {
  const names: string[] = [];
  const taskIds: string[] = [];
  for (let i = 0; i < 7; i++) {
    const name = `sidebar-reuse-organization-long-repository-${i}`;
    const dir = mkdtempSync(path.join(path.dirname(seed.repositoryPath), "sidebar-reuse-"));
    directories.push(dir);
    execFileSync("git", ["clone", "--local", seed.repositoryPath, dir], { stdio: "ignore" });
    const repo = await api.createRepository(seed.workspaceId, dir, "main", { name });
    const task = await api.createTask(seed.workspaceId, `Cached repository task ${i}`, {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [repo.id],
    });
    names.push(name);
    taskIds.push(task.id);
  }
  const other = await api.createTask(seed.workspaceId, "Other view task", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const current = await api.createTask(seed.workspaceId, "Open conversation", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const { session_id } = await api.seedTaskSession(current.id, {
    state: "COMPLETED",
    agentProfileId: seed.agentProfileId,
    completedAt: new Date().toISOString(),
  });
  await api.seedSessionMessage(session_id, {
    type: "message",
    content: "View switching preserves this conversation",
  });
  await api.updateTaskState(current.id, "COMPLETED");
  await api.archiveTask(current.id);
  await Promise.all([...taskIds, other.id].map((id) => api.archiveTask(id)));
  const views = [
    {
      id: "reuse-a",
      name: "Repositories",
      filters: [{ id: "repo", dimension: "repository", op: "in", value: names }],
    },
    {
      id: "reuse-b",
      name: "Other",
      filters: [{ id: "title", dimension: "titleMatch", op: "matches", value: "Other view task" }],
    },
  ].map((view) => ({
    ...view,
    filters: [...view.filters, { id: "archive", dimension: "archived", op: "is", value: true }],
    sort: { key: "updatedAt", direction: "desc" },
    group: "none",
    collapsed_groups: [],
  }));
  const saved = await api.rawRequest("PATCH", "/api/v1/user/settings", {
    sidebar_view_state: {
      workspace_id: seed.workspaceId,
      views,
      active_view_id: views[0].id,
      draft: null,
    },
  });
  expect(saved.ok).toBe(true);
  expect(JSON.stringify(names).length).toBeGreaterThan(256);
  return { current, other, taskIds };
}

// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.14
// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.15
// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.17
async function verifySidebarViewReuse(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
  options: { directories: string[]; capture?: PrAssetCapture },
) {
  const { directories, capture } = options;
  const { current, other, taskIds } = await seedViews(api, seed, directories);
  await page.goto(`/t/${current.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  if (mobile) await page.getByTestId("mobile-task-picker-trigger").tap();
  const surface = mobile ? page.getByRole("dialog", { name: "Tasks" }) : session.sidebar;
  // Real backend membership proves the previously rejected selection loads.
  for (const id of taskIds)
    await expect(surface.locator(`[data-task-row-id="${id}"]`)).toBeVisible();
  const refreshStatus = surface.getByRole("status").filter({ hasText: "Updating tasks" });
  await expect(refreshStatus).toHaveCount(0);
  // The view sorts newest first; lower rows can grow as their metadata hydrates.
  const firstTaskId = taskIds.at(-1)!;
  const settledRowTop = await taskRowTop(surface, firstTaskId);
  const viewControls = mobile
    ? {
        selectViewByName: async (name: string) => {
          await surface.getByRole("button", { name, exact: true }).tap();
        },
      }
    : new SidebarFilterPopoverPage(page);
  const filters = {
    selectViewByName: async (name: string) => {
      const activeViewId = name === "Repositories" ? "reuse-a" : "reuse-b";
      // Rows update optimistically; a later reload needs the saved preference.
      const saved = page.waitForResponse((response) => {
        const request = response.request();
        if (
          request.method() !== "PATCH" ||
          new URL(response.url()).pathname !== "/api/v1/user/settings"
        )
          return false;
        const selection = request.postDataJSON().sidebar_view_state;
        return (
          selection?.workspace_id === seed.workspaceId && selection.active_view_id === activeViewId
        );
      });
      await viewControls.selectViewByName(name);
      expect((await saved).ok()).toBe(true);
    },
  };
  const loadedB = waitForHttp(page, "POST", /\/sidebar\/query$/);
  await filters.selectViewByName("Other");
  await loadedB;
  await expect(surface.locator(`[data-task-row-id="${other.id}"]`)).toBeVisible();

  const arrived = gate(),
    release = gate();
  const refreshFailures = sidebarFailureBarrier();
  let failRefresh = false;
  await page.bringToFront();
  await page.route("**/sidebar/query", async (route) => {
    const query = route.request().postDataJSON();
    if (query.filters?.[0]?.dimension !== "repository") return route.continue();
    arrived.release();
    await release.promise;
    if (failRefresh) {
      refreshFailures.count(query);
      return route.fulfill({
        status: 503,
        contentType: "application/json",
        body: '{"error":"private internal failure"}',
      });
    }
    return route.continue();
  });
  const refreshed = waitForHttp(page, "POST", /\/sidebar\/query$/);
  try {
    await filters.selectViewByName("Repositories");
    await arrived.promise;
    for (const id of taskIds)
      await expect(surface.locator(`[data-task-row-id="${id}"]`)).toBeVisible();
    await expect(surface.locator(`[data-task-row-id="${other.id}"]`)).toHaveCount(0);
    await expect(refreshStatus).toContainText("Updating tasks");
    expect(await taskRowTop(surface, firstTaskId)).toBeCloseTo(settledRowTop, 0);
    await expect(page).toHaveURL(new RegExp(`/t/${current.id}$`));
    await capture?.screenshot(mobile ? "phone-background-refresh" : "desktop-background-refresh", {
      caption: "Background refresh keeps the saved-view task rows in place.",
    });
    failRefresh = true;
    release.release();
    await refreshed;
    await expect(surface.getByRole("alert")).toHaveCount(0);
    await expect(surface.locator("[data-task-row-id]")).toHaveCount(taskIds.length);
    await refreshFailures.exhausted;
    await expect(surface.getByRole("alert")).toHaveCount(1);
    await expect(surface.getByRole("alert")).toContainText("Tasks could not be refreshed");
    await expect(surface).not.toContainText("private internal failure");
    for (const id of taskIds)
      await expect(surface.locator(`[data-task-row-id="${id}"]`)).toBeVisible();
    await capture?.screenshot(mobile ? "phone-refresh-error" : "desktop-refresh-error", {
      caption:
        "A failed background refresh preserves the selected view and offers one Retry action.",
    });
    failRefresh = false;
    const retry = surface.getByRole("button", { name: "Retry", exact: true });
    if (mobile) expect((await retry.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await retrySidebarRecovery(page, surface);
    await expect(refreshStatus).toHaveCount(0);
    expect(await taskRowTop(surface, firstTaskId)).toBeCloseTo(settledRowTop, 0);
  } finally {
    release.release();
    await Promise.allSettled([refreshed]);
    await page.unrouteAll({ behavior: "wait" });
  }

  const invalidated = waitForHttp(page, "POST", /\/sidebar\/query$/);
  await api.unarchiveTask(taskIds[0]);
  await invalidated;
  await expect(surface.locator(`[data-task-row-id="${taskIds[0]}"]`)).toHaveCount(0);
  await filters.selectViewByName("Other");
  await expect(surface.locator(`[data-task-row-id="${other.id}"]`)).toBeVisible();
  await filters.selectViewByName("Repositories");
  await expect(surface.locator(`[data-task-row-id="${taskIds[0]}"]`)).toHaveCount(0);
  await verifyQueryRejections(page, surface, filters, taskIds.slice(1));
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  const initialFailures = sidebarFailureBarrier();
  await page.bringToFront();
  await page.route("**/sidebar/query", (route) => {
    initialFailures.count(route.request().postDataJSON());
    return route.fulfill({
      status: 503,
      contentType: "application/json",
      body: '{"error":"private initial failure"}',
    });
  });
  try {
    await page.reload();
    await session.waitForLoad();
    if (mobile) await page.getByTestId("mobile-task-picker-trigger").tap();
    await expect(surface.locator("[data-task-row-id]")).toHaveCount(0);
    await expect(surface.getByRole("alert")).toHaveCount(0);
    await initialFailures.exhausted;
    await expect(surface.getByRole("alert")).toHaveCount(1);
    await expect(surface.getByRole("alert")).toContainText("Could not load this page");
    await expect(surface.locator("[data-task-row-id]")).toHaveCount(0);
    await expect(surface).not.toContainText("private initial failure");
  } finally {
    await page.unrouteAll({ behavior: "wait" });
  }
  await retrySidebarRecovery(page, surface);
  for (const id of taskIds.slice(1))
    await expect(surface.locator(`[data-task-row-id="${id}"]`)).toBeVisible();
  if (mobile) {
    await page.keyboard.press("Escape");
    await expect(surface).toBeHidden();
    await expect(page.getByTestId("mobile-task-picker-trigger")).toBeFocused();
  }
  await expect(
    session.activeChat().getByText("View switching preserves this conversation").last(),
  ).toBeVisible();
}

async function verifyQueryRejections(
  page: Page,
  surface: ReturnType<Page["locator"]>,
  filters: { selectViewByName: (name: string) => Promise<void> },
  ids: string[],
) {
  let status = 400;
  await page.route("**/sidebar/query", (route) => {
    if (route.request().postDataJSON().filters?.[0]?.dimension !== "repository")
      return route.continue();
    return route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify({
        error: "private rejected value",
        error_code: "sidebar_query_invalid",
        details: { reason: "list_count", filter_index: 0, limit: 1000 },
      }),
    });
  });
  try {
    await filters.selectViewByName("Other");
    await expect(surface.getByRole("alert")).toHaveCount(0);
    await filters.selectViewByName("Repositories");
    await expect(surface.getByRole("alert")).toHaveCount(1);
    await expect(surface.getByRole("alert")).toContainText("1000");
    await expect(surface.getByRole("button", { name: "Retry", exact: true })).toHaveCount(0);
    await expect(surface).not.toContainText("private rejected value");
    status = 403;
    await filters.selectViewByName("Other");
    await expect(surface.getByRole("alert")).toHaveCount(0);
    await filters.selectViewByName("Repositories");
    await expect(surface.getByRole("alert")).toHaveCount(1);
    for (const id of ids)
      await expect(surface.locator(`[data-task-row-id="${id}"]`)).toHaveCount(0);
    await expect(surface.getByRole("button", { name: "Retry", exact: true })).toHaveCount(0);
  } finally {
    await page.unrouteAll({ behavior: "wait" });
  }
  await page.reload();
  await new SessionPage(page).waitForLoad();
  const mobilePicker = page.getByTestId("mobile-task-picker-trigger");
  if (await mobilePicker.isVisible()) await mobilePicker.tap();
  await filters.selectViewByName("Other");
  await filters.selectViewByName("Repositories");
  for (const id of ids) await expect(surface.locator(`[data-task-row-id="${id}"]`)).toBeVisible();
}

export async function exerciseSidebarViewReuse(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
  capture?: PrAssetCapture,
) {
  const { settings } = await api.getUserSettings();
  const baseline = settings.sidebar_views_by_workspace?.[seed.workspaceId];
  const directories: string[] = [];
  try {
    await verifySidebarViewReuse(page, api, seed, mobile, { directories, capture });
  } finally {
    const restored = await api.rawRequest("PATCH", "/api/v1/user/settings", {
      sidebar_view_state: {
        workspace_id: seed.workspaceId,
        views: baseline?.views ?? [],
        active_view_id: baseline?.active_view_id ?? "all",
        draft: baseline?.draft ?? null,
      },
    });
    expect(restored.ok).toBe(true);
    for (const directory of directories) rmSync(directory, { recursive: true, force: true });
  }
}
