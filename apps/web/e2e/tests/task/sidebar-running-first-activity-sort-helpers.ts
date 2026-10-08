import { expect, type Locator, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import type { ApiClient } from "../../helpers/api-client";
import { touchDragBetween } from "../../helpers/touch-drag";
import type { WsFrame, WsWatcher } from "../../helpers/causal-waits";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import type {
  SidebarTaskColorPatchApi,
  SidebarTaskColorsApi,
} from "../../../lib/types/http-user-settings";

type SidebarSortScenario = {
  parent: { id: string; title: string };
  child: { id: string; title: string };
  runningBlue: { id: string; title: string };
  idleRed: { id: string; title: string };
  idleBlue: { id: string; title: string };
  rootIds: string[];
  taskIds: string[];
  colorIds: string[];
};

type SidebarViewState = {
  views: Array<Record<string, unknown>>;
  active_view_id: string;
  draft: unknown;
};

export type SidebarRunningRankScenario = {
  runningNoPrimary: { id: string; title: string };
  secondaryTask: { id: string; title: string };
  idleRed: { id: string; title: string };
  idleOrange: { id: string; title: string };
  primarySessionId: string;
  secondarySessionId: string;
  runningNoPrimarySessionId: string;
  rootIds: string[];
  taskIds: string[];
  colorIds: string[];
};

export type SidebarColorPatchSnapshot = SidebarTaskColorPatchApi;

export async function readPreviousSidebarViewState(
  api: ApiClient,
  workspaceId: string,
): Promise<SidebarViewState | undefined> {
  const { settings } = await api.getUserSettings();
  return settings.sidebar_views_by_workspace[workspaceId];
}

export async function restoreSidebarViewState(
  api: ApiClient,
  workspaceId: string,
  previous: SidebarViewState | undefined,
): Promise<void> {
  await api.saveUserSettings({
    sidebar_view_state: {
      workspace_id: workspaceId,
      ...(previous ?? { views: [], active_view_id: "", draft: null }),
    },
  });
}

export async function readPreviousSidebarColorPatch(
  api: ApiClient,
): Promise<SidebarColorPatchSnapshot | undefined> {
  const { settings } = await api.getUserSettings();
  const colors = settings.sidebar_task_colors as SidebarTaskColorsApi | undefined;
  return colors ? { colors: { ...colors }, if_missing: false } : undefined;
}

export async function restoreSidebarSortColors(
  api: ApiClient,
  taskIds: string[],
  previous: SidebarColorPatchSnapshot | undefined,
): Promise<void> {
  if (taskIds.length === 0) return;
  await api.saveUserSettings({
    sidebar_task_color_patch: {
      colors: Object.fromEntries(taskIds.map((id) => [id, previous?.colors[id] ?? null])),
      if_missing: previous?.if_missing ?? false,
    },
  });
}

export async function seedSidebarRunningRankScenario(
  api: ApiClient,
  seed: SeedData,
  prefix: string,
): Promise<SidebarRunningRankScenario> {
  const taskOptions = {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  };
  const runningNoPrimary = await api.createTask(
    seed.workspaceId,
    `${prefix} running without primary`,
    taskOptions,
  );
  const secondaryTask = await api.createTask(
    seed.workspaceId,
    `${prefix} waiting primary with secondary`,
    taskOptions,
  );
  const idleRed = await api.createTask(seed.workspaceId, `${prefix} idle red`, taskOptions);
  const idleOrange = await api.createTask(seed.workspaceId, `${prefix} idle orange`, taskOptions);

  const { session_id: runningNoPrimarySessionId } = await api.seedTaskSession(runningNoPrimary.id, {
    state: "RUNNING",
    sessionId: `sidebar-running-no-primary-${runningNoPrimary.id}`,
    startedAt: new Date(Date.now() - 10 * 60_000).toISOString(),
  });
  const { session_id: primarySessionId } = await api.seedTaskSession(secondaryTask.id, {
    state: "WAITING_FOR_INPUT",
    sessionId: `sidebar-running-primary-${secondaryTask.id}`,
    agentProfileId: seed.agentProfileId,
    startedAt: new Date(Date.now() - 9 * 60_000).toISOString(),
  });
  const { session_id: secondarySessionId } = await api.seedTaskSession(secondaryTask.id, {
    state: "WAITING_FOR_INPUT",
    sessionId: `sidebar-running-secondary-${secondaryTask.id}`,
    startedAt: new Date(Date.now() - 8 * 60_000).toISOString(),
  });
  await api.setPrimarySession(primarySessionId);

  // Newer idle peers deliberately win the color and activity tie-breakers.
  await api.updateTaskTitle(runningNoPrimary.id, `${prefix} running no primary`);
  await api.updateTaskTitle(secondaryTask.id, `${prefix} waiting primary, secondary running`);
  await api.updateTaskTitle(idleRed.id, `${prefix} idle red`);
  await api.updateTaskTitle(idleOrange.id, `${prefix} idle orange`);

  return {
    runningNoPrimary,
    secondaryTask,
    idleRed,
    idleOrange,
    primarySessionId,
    secondarySessionId,
    runningNoPrimarySessionId,
    rootIds: [runningNoPrimary.id, secondaryTask.id, idleRed.id, idleOrange.id],
    taskIds: [runningNoPrimary.id, secondaryTask.id, idleRed.id, idleOrange.id],
    colorIds: [runningNoPrimary.id, secondaryTask.id, idleRed.id, idleOrange.id],
  };
}

export async function saveSidebarRunningRankView(
  api: ApiClient,
  seed: SeedData,
  viewId: string,
  token: string,
): Promise<void> {
  await api.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seed.workspaceId,
      views: [
        {
          id: viewId,
          name: `${token} view`,
          filters: [{ id: "sort-token", dimension: "titleMatch", op: "matches", value: token }],
          sort: {
            key: "running",
            direction: "desc",
            then_by: [
              { key: "color", color: "red", direction: "desc" },
              { key: "color", color: "orange", direction: "desc" },
              { key: "lastActivityAt", direction: "desc" },
            ],
          },
          group: "workflowStep",
          collapsed_groups: [],
        },
      ],
      active_view_id: viewId,
      draft: null,
    },
  });
}

export async function readTaskRunningSummary(
  api: ApiClient,
  workspaceId: string,
  taskId: string,
): Promise<{ has_running_session?: boolean; revision: number } | undefined> {
  const { tasks } = await api.listTasks(workspaceId);
  return tasks.find((task) => task.id === taskId)?.status_summary ?? undefined;
}

export function waitForTaskRunningSummary(
  watcher: WsWatcher,
  taskId: string,
  expected: boolean,
  afterRevision: number,
): Promise<WsFrame> {
  return watcher.waitForEvent("task.status_summary.updated", {
    where: (payload) => {
      const summary = payload.status_summary as
        | { has_running_session?: unknown; revision?: unknown }
        | undefined;
      return (
        payload.task_id === taskId &&
        summary?.has_running_session === expected &&
        typeof summary.revision === "number" &&
        summary.revision > afterRevision
      );
    },
  });
}

export async function seedSidebarSortScenario(
  api: ApiClient,
  seed: SeedData,
  prefix: string,
): Promise<SidebarSortScenario> {
  const taskOptions = {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  };
  const parent = await api.createTask(
    seed.workspaceId,
    `${prefix} running red parent`,
    taskOptions,
  );
  const child = await api.createTask(seed.workspaceId, `${prefix} running child`, {
    ...taskOptions,
    parent_id: parent.id,
  });
  const runningBlue = await api.createTask(seed.workspaceId, `${prefix} running blue`, taskOptions);
  const idleRed = await api.createTask(seed.workspaceId, `${prefix} idle red`, taskOptions);
  const idleBlue = await api.createTask(seed.workspaceId, `${prefix} idle blue`, taskOptions);

  const { session_id: childSessionId } = await api.seedTaskSession(child.id, {
    state: "RUNNING",
    agentProfileId: seed.agentProfileId,
  });
  await api.setPrimarySession(childSessionId);
  const { session_id: runningSessionId } = await api.seedTaskSession(runningBlue.id, {
    state: "RUNNING",
    agentProfileId: seed.agentProfileId,
  });
  await api.setPrimarySession(runningSessionId);
  await api.saveUserSettings({
    sidebar_task_color_patch: {
      colors: {
        [parent.id]: "red",
        [child.id]: "blue",
        [runningBlue.id]: "blue",
        [idleRed.id]: "red",
        [idleBlue.id]: "blue",
      },
      if_missing: false,
    },
  });

  // Build a stable activity order that differs from both running and color order.
  await api.updateTaskTitle(idleRed.id, `${prefix} idle red older activity`);
  await api.updateTaskTitle(child.id, `${prefix} running child recent activity`);
  await api.updateTaskTitle(runningBlue.id, `${prefix} running blue newer activity`);
  await api.updateTaskTitle(idleBlue.id, `${prefix} idle blue newest activity`);

  return {
    parent,
    child,
    runningBlue,
    idleRed,
    idleBlue,
    rootIds: [parent.id, runningBlue.id, idleRed.id, idleBlue.id],
    taskIds: [parent.id, child.id, runningBlue.id, idleRed.id, idleBlue.id],
    colorIds: [parent.id, child.id, runningBlue.id, idleRed.id, idleBlue.id],
  };
}

export async function saveSidebarSortView(
  api: ApiClient,
  seed: SeedData,
  viewId: string,
  token: string,
) {
  await api.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seed.workspaceId,
      views: [
        {
          id: viewId,
          name: `${token} view`,
          filters: [{ id: "sort-token", dimension: "titleMatch", op: "matches", value: token }],
          sort: {
            key: "running",
            direction: "desc",
            then_by: [{ key: "lastActivityAt", direction: "desc" }],
          },
          group: "workflowStep",
          collapsed_groups: [],
        },
      ],
      active_view_id: viewId,
      draft: null,
    },
  });
}

export async function openSidebarSortEditor(page: Page, mobile: boolean) {
  if (mobile) {
    await page.getByTestId("mobile-task-picker-trigger").tap();
    await expect(page.getByRole("dialog", { name: "Tasks" })).toBeVisible();
  }
  const filters = new SidebarFilterPopoverPage(page);
  await filters.open();
  return { filters, popover: filters.popover };
}

type SidebarSortRule = { key?: string; color?: string; thenBy?: SidebarSortRule[] };
type SidebarWorkspaceState = {
  activeViewId: string;
  draft: { sort?: SidebarSortRule } | null;
  syncPending?: boolean;
  views: Array<{ id: string; sort: SidebarSortRule }>;
};

function matchesSidebarSort(
  sidebar: SidebarWorkspaceState | null,
  expectedKeys: string[],
  expectedColor?: string,
): boolean {
  if (!sidebar || sidebar.syncPending === true) return false;
  const activeView = sidebar.views.find((view) => view.id === sidebar.activeViewId);
  const sort = sidebar.draft?.sort ?? activeView?.sort;
  if (!sort) return false;
  const rules = [{ key: sort.key, color: sort.color }, ...(sort.thenBy ?? [])];
  if (rules.length !== expectedKeys.length) return false;
  if (!rules.every((rule, index) => rule.key === expectedKeys[index])) return false;
  if (expectedColor === undefined) return true;
  return rules.some((rule) => rule.key === "color" && rule.color === expectedColor);
}

export async function waitForSidebarSortSync(
  page: Page,
  workspaceId: string,
  expectedKeys: string[],
  expectedColor?: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const sidebar = await page.evaluate((id) => {
          const store = (
            window as Window & {
              __KANDEV_E2E_STORE__?: {
                getState: () => {
                  sidebarViewsByWorkspace?: Record<string, SidebarWorkspaceState>;
                };
              };
            }
          ).__KANDEV_E2E_STORE__;
          return store?.getState().sidebarViewsByWorkspace?.[id] ?? null;
        }, workspaceId);
        return matchesSidebarSort(sidebar, expectedKeys, expectedColor);
      },
      { timeout: 10_000 },
    )
    .toBe(true);
}

export async function chooseSortField(
  page: Page,
  popover: Locator,
  testId: string,
  label: string,
): Promise<void> {
  await popover.getByTestId(testId).click();
  await page.getByRole("option", { name: label, exact: true }).click();
}

export async function addColorAfterActivity(page: Page, popover: Locator): Promise<void> {
  await popover.getByTestId("sort-add-rule-button").click();
  await chooseSortField(page, popover, "sort-rule-key-2", "Color");
  await popover.getByTestId("sort-rule-color-2").click();
  await page.getByRole("option", { name: "Red", exact: true }).click();
  await moveSortRuleWithMenu(page, popover, 3, "up");
  await expect(popover.getByTestId("sort-key-select")).toContainText("Running");
  await expect(popover.getByTestId("sort-rule-key-1")).toContainText("Color");
  await expect(popover.getByTestId("sort-rule-key-2")).toContainText("Last activity");
}

export async function moveSortRuleWithMenu(
  page: Page,
  popover: Locator,
  position: number,
  direction: "up" | "down",
): Promise<void> {
  const trigger = popover.getByTestId(`sort-rule-more-${position - 1}`);
  const move = page.getByTestId(`sort-rule-more-${position - 1}-move-${direction}`);
  const menu = page.getByRole("menu");
  if (await menu.count()) await expect(menu.first()).toBeHidden();
  if (await page.evaluate(() => matchMedia("(pointer: coarse)").matches)) {
    await trigger.tap();
    await expect(move).toBeVisible();
    await move.tap();
  } else {
    await trigger.click();
    await expect(move).toBeVisible();
    await move.click();
  }
  await expect(menu).toBeHidden();
}

export async function touchDragSortRule(
  page: Page,
  source: Locator,
  target: Locator,
): Promise<void> {
  const targetTestId = await target.getAttribute("data-testid");
  expect(targetTestId).not.toBeNull();
  const sourceList = source.locator("xpath=ancestor::*[@data-sidebar-reorder-list][1]");
  await expect(sourceList).toHaveCount(1);
  const scopedTarget = sourceList.getByTestId(targetTestId!);
  await expect(scopedTarget).toHaveCount(1);
  await scopedTarget.scrollIntoViewIfNeeded();
  await source.scrollIntoViewIfNeeded();
  await waitForFiniteAnimations(sourceList);
  await expect(source).toBeInViewport();
  await expect(scopedTarget).toBeInViewport();
  const sourceBox = await source.boundingBox();
  const targetBox = await scopedTarget.boundingBox();
  const listTop = await sourceList.evaluate((element) => element.getBoundingClientRect().top);
  expect(sourceBox).not.toBeNull();
  expect(targetBox).not.toBeNull();
  expect(listTop).not.toBeNull();
  const end = {
    x: targetBox!.x + targetBox!.width / 2,
    y: Math.max(targetBox!.y + 2, listTop! + 40),
  };
  await touchDragBetween(
    page,
    { x: sourceBox!.x + sourceBox!.width / 2, y: sourceBox!.y + sourceBox!.height / 2 },
    end,
    async () => {
      await expect(page.locator('[data-dragging="true"]')).toHaveCount(1);
    },
  );
}

export async function sidebarRootOrder(surface: Locator, ids: string[]): Promise<string[]> {
  const rows = await surface
    .locator("[data-task-row-id]")
    .evaluateAll((elements) => elements.map((element) => element.getAttribute("data-task-row-id")));
  return rows.filter((id): id is string => id !== null && ids.includes(id));
}

export async function expectSidebarRootOrder(
  surface: Locator,
  ids: string[],
  expected: string[],
): Promise<void> {
  await expect
    .poll(async () => (await sidebarRootOrder(surface, ids)).slice(0, expected.length))
    .toEqual(expected);
}

export async function cleanupSidebarSortColors(api: ApiClient, colorIds: string[]): Promise<void> {
  if (colorIds.length === 0) return;
  await api.saveUserSettings({
    sidebar_task_color_patch: {
      colors: Object.fromEntries(colorIds.map((id) => [id, null])),
      if_missing: false,
    },
  });
}
