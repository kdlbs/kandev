import { expect, type Page, type Locator } from "@playwright/test";
import type { StoreApi } from "zustand";
import type { AppState } from "../../../lib/state/store";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import {
  seedSidebarPaginationTasks,
  archiveSidebarPaginationTasks,
} from "./sidebar-task-pagination-fixtures";

const ARCHIVES = "Shared archives";
const ACTIVE = "Shared active";
function gate() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { promise, release };
}
async function saveViews(api: ApiClient, seed: SeedData, archived: boolean) {
  await api.saveUserSettings({ enable_preview_on_click: false });
  const views = [false, true].map((archived) => ({
    id: archived ? "shared-archive" : "shared-active",
    name: archived ? ARCHIVES : ACTIVE,
    filters: [
      { id: "archive", dimension: "archived", op: "is", value: archived },
      { id: "title", dimension: "titleMatch", op: "matches", value: "Shared fixture" },
    ],
    sort: { key: "title", direction: "asc" },
    group: "state",
    collapsed_groups: [],
  }));
  await api.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seed.workspaceId,
      views,
      active_view_id: views[Number(archived)].id,
      draft: null,
    },
  });
}
export async function anchorTask(api: ApiClient, seed: SeedData) {
  const task = await api.createTask(seed.workspaceId, "Conversation anchor", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const { session_id } = await api.seedTaskSession(task.id, {
    state: "COMPLETED",
    agentProfileId: seed.agentProfileId,
    completedAt: new Date().toISOString(),
  });
  await api.seedSessionMessage(session_id, {
    type: "message",
    content: "Shared sidebar conversation stays open",
  });
  return task;
}
export async function surface(page: Page, mobile: boolean) {
  const session = new SessionPage(page);
  await session.waitForLoad();
  if (mobile) await page.getByTestId("mobile-task-picker-trigger").tap();
  const rows = mobile ? page.getByRole("dialog", { name: "Tasks", exact: true }) : session.sidebar;
  await expect(rows).toBeVisible();
  return { session, rows };
}
async function selectView(page: Page, rows: Locator, mobile: boolean, name: string) {
  if (mobile) await rows.getByRole("button", { name, exact: true }).tap();
  else await new SidebarFilterPopoverPage(page).selectViewByName(name);
}
export async function waitForCoverage(page: Page, workspaceId: string) {
  await expect
    .poll(() =>
      page.evaluate((workspaceId) => {
        const state = (
          window as Window & { __KANDEV_E2E_STORE__: StoreApi<AppState> }
        ).__KANDEV_E2E_STORE__.getState();
        const coverage = state.workflows.taskWorkflowCoverage;
        return (
          coverage?.workspace_id === workspaceId &&
          coverage.complete &&
          coverage.workflow_ids.every(
            (id) => state.kanbanMulti.snapshots[id]?.taskCoverage?.complete,
          )
        );
      }, workspaceId),
    )
    .toBe(true);
}

export async function exerciseSharedPaging(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  await saveViews(api, seed, false);
  const active = await seedSidebarPaginationTasks(api, seed, "Shared fixture active");
  const archived = await seedSidebarPaginationTasks(api, seed, "Shared fixture archived");
  await archiveSidebarPaginationTasks(api, archived);
  const anchor = await anchorTask(api, seed);
  let requests = 0;
  page.on("request", (request) => {
    if (request.url().endsWith("/sidebar/query")) requests++;
  });
  await page.goto("/");
  await waitForCoverage(page, seed.workspaceId);
  expect(requests).toBe(0);
  if (mobile) {
    const home = new MobileKanbanPage(page);
    await home.openSearch();
    await home.searchInput().fill(anchor.title);
    await home.taskCard(anchor.id).tap();
  } else {
    await page
      .getByTestId("kanban-header-search")
      .getByPlaceholder("Search tasks...", { exact: true })
      .fill(anchor.title);
    await page.getByTestId(`task-card-${anchor.id}`).click();
  }
  const { session, rows } = await surface(page, mobile);
  const controls = rows.getByTestId("sidebar-page-controls");
  await expect(rows.locator("[data-task-row-id]")).toHaveCount(100);
  await expect(rows.getByRole("status").filter({ hasText: "Updating tasks" })).toHaveCount(0);
  await controls.getByRole("button").last().click();
  await expect(controls.getByText("Page 2 of 2")).toBeVisible();
  await expect(rows.locator("[data-task-row-id]")).toHaveCount(1);
  expect(requests).toBe(0);
  await selectView(page, rows, mobile, ARCHIVES);
  await expect(rows.locator("[data-task-row-id]")).toHaveCount(100);
  expect(requests).toBe(1);
  for (let index = 0; index < 3; index++) {
    await controls.getByRole("button").last().click();
    await expect(controls.getByText("Page 2 of 2")).toBeVisible();
    await expect(rows.locator("[data-task-row-id]")).toHaveCount(1);
    const owners = await page.evaluate(() => {
      const state = (
        window as Window & { __KANDEV_E2E_STORE__: StoreApi<AppState> }
      ).__KANDEV_E2E_STORE__.getState();
      return {
        entities: Object.keys(state.taskOverview.byId).length,
        displayed: Object.entries(state.taskOverview.owners)
          .filter(([key]) => key.startsWith("sidebar:display:"))
          .flatMap(([, ids]) => ids).length,
      };
    });
    expect(owners.entities).toBeLessThanOrEqual(active.length + 102);
    expect(owners.displayed).toBeLessThanOrEqual(2);
    await controls.getByRole("button").first().click();
    await expect(controls.getByText("Page 1 of 2")).toBeVisible();
    await expect(rows.locator("[data-task-row-id]")).toHaveCount(100);
  }
  await expect(page).toHaveURL(new RegExp(`/t/${anchor.id}$`));
  if (mobile) {
    const viewport = page.viewportSize()!;
    const bounds = (await rows.boundingBox())!;
    expect(bounds.x).toBeGreaterThanOrEqual(0);
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(viewport.width + 1);
    await page.keyboard.press("Escape");
    await expect(page.getByTestId("mobile-task-picker-trigger")).toBeFocused();
  }
  await expect(session.activeChat()).toContainText("Shared sidebar conversation stays open");
}

export async function exerciseSharedFirstResponse(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  await saveViews(api, seed, true);
  const tasks = [];
  for (const name of ["kept", "deleted", "unarchived"])
    tasks.push(
      await api.createTask(seed.workspaceId, `Shared fixture ${name}`, {
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
      }),
    );
  await archiveSidebarPaginationTasks(
    api,
    tasks.map((task) => task.id),
  );
  const anchor = await anchorTask(api, seed);
  const captured = gate(),
    first = gate(),
    trailing = gate();
  let requests = 0;
  await page.route("**/sidebar/query", async (route) => {
    requests++;
    if (requests === 1) {
      const response = await route.fetch();
      captured.release();
      await first.promise;
      await route.fulfill({ response });
    } else {
      await trailing.promise;
      await route.continue();
    }
  });
  try {
    await page.goto(`/t/${anchor.id}`);
    const { rows } = await surface(page, mobile);
    await captured.promise;
    await api.updateTaskTitle(tasks[0].id, "Shared fixture kept live");
    await api.deleteTask(tasks[1].id);
    await api.unarchiveTask(tasks[2].id);
    await expect
      .poll(() =>
        page.evaluate(
          (ids) => {
            const overview = (
              window as Window & { __KANDEV_E2E_STORE__: StoreApi<AppState> }
            ).__KANDEV_E2E_STORE__.getState().taskOverview;
            return Object.values(overview.reads).some((read) =>
              ids.every((id) => Object.hasOwn(read.changes, id)),
            );
          },
          tasks.map((task) => task.id),
        ),
      )
      .toBe(true);
    expect(requests).toBe(1);
    first.release();
    await expect(rows.locator(`[data-task-row-id="${tasks[0].id}"]`)).toContainText("kept live");
    for (const task of tasks.slice(1))
      await expect(rows.locator(`[data-task-row-id="${task.id}"]`)).toHaveCount(0);
    await expect.poll(() => requests).toBe(2);
    await expect(page).toHaveURL(new RegExp(`/t/${anchor.id}$`));
    trailing.release();
    await expect(rows.getByRole("status").filter({ hasText: "Updating tasks" })).toHaveCount(0);
    expect(requests).toBe(2);
  } finally {
    first.release();
    trailing.release();
    await page.unrouteAll({ behavior: "wait" });
  }
}
