import { expect, type Locator, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";

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
  await popover.getByTestId("sort-rule-up-2").click();
  await expect(popover.getByTestId("sort-key-select")).toContainText("Running");
  await expect(popover.getByTestId("sort-rule-key-1")).toContainText("Color");
  await expect(popover.getByTestId("sort-rule-key-2")).toContainText("Last activity");
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
