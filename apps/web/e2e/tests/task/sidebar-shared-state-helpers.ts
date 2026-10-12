import { expect, type Page } from "@playwright/test";
import type { StoreApi } from "zustand";
import type { AppState } from "../../../lib/state/store";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";

async function seedSharedViews(api: ApiClient, seed: SeedData) {
  const tasks = [];
  for (const title of ["Shared sidebar anchor", "Shared sidebar destination"]) {
    const task = await api.createTask(seed.workspaceId, title, {
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
      content: `Conversation for ${title}`,
    });
    tasks.push(task);
  }
  await api.saveUserSettings({ enable_preview_on_click: false });
  const views = [
    { id: "shared-all", name: "Shared tasks", filters: [] },
    {
      id: "shared-destination",
      name: "Destination",
      filters: [{ id: "title", dimension: "titleMatch", op: "matches", value: "destination" }],
    },
  ].map((view) => ({
    ...view,
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
  return tasks;
}

async function waitForSharedInventory(page: Page, workspaceId: string) {
  await expect
    .poll(() =>
      page.evaluate((id) => {
        const state = (
          window as Window & { __KANDEV_E2E_STORE__?: StoreApi<AppState> }
        ).__KANDEV_E2E_STORE__?.getState();
        if (!state || state.workspaceContextRead.workspaceId !== id) return false;
        const workflows = state.workflows.items.filter((workflow) => workflow.workspaceId === id);
        return (
          workflows.length > 0 &&
          workflows.every((workflow) => {
            const snapshot = state.kanbanMulti.snapshots[workflow.id];
            return snapshot && !snapshot.isPlaceholder && !snapshot.fetchFailed;
          })
        );
      }, workspaceId),
    )
    .toBe(true);
}

// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.19 AC-UI-SIDEBAR-ARCHIVED-FILTER-002.20
export async function exerciseSharedSidebarState(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const tasks = await seedSharedViews(api, seed);
  const [anchor, destination] = tasks;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const forwards: Promise<void>[] = [];
  let queryCount = 0;
  await page.route("**/sidebar/query", (route) => {
    queryCount += 1;
    const forward = gate.then(() => route.continue()).catch(() => undefined);
    forwards.push(forward);
    return forward;
  });
  try {
    await page.goto("/");
    await waitForSharedInventory(page, seed.workspaceId);
    await page.getByTestId(`task-card-${anchor.id}`).click();
    await expect(page).toHaveURL(new RegExp(`/t/${anchor.id}$`));
    const session = new SessionPage(page);
    await session.waitForLoad();
    await expect(session.activeChat()).toContainText(`Conversation for ${anchor.title}`);
    if (mobile) await page.getByTestId("mobile-task-picker-trigger").tap();
    const surface = mobile
      ? page.getByRole("dialog", { name: "Tasks", exact: true })
      : session.sidebar;
    await expect(surface.locator(`[data-task-row-id="${destination.id}"]`)).toBeVisible();
    const queriesBeforeSwitch = queryCount;
    if (mobile) await surface.getByRole("button", { name: "Destination", exact: true }).tap();
    else await new SidebarFilterPopoverPage(page).selectViewByName("Destination");
    await expect(surface.locator(`[data-task-row-id="${anchor.id}"]`)).toHaveCount(0);
    await expect(surface.locator(`[data-task-row-id="${destination.id}"]`)).toBeVisible();
    await expect(surface.getByTestId("sidebar-page-controls")).toBeHidden();
    await api.archiveTask(destination.id);
    await expect(surface.locator(`[data-task-row-id="${destination.id}"]`)).toHaveCount(0);
    await api.unarchiveTask(destination.id);
    await expect(surface.locator(`[data-task-row-id="${destination.id}"]`)).toBeVisible();
    expect(queryCount).toBe(queriesBeforeSwitch);
    await expect(page).toHaveURL(new RegExp(`/t/${anchor.id}$`));
    if (mobile) {
      await page.keyboard.press("Escape");
      await expect(page.getByTestId("mobile-task-picker-trigger")).toBeFocused();
    }
    await expect(session.activeChat()).toContainText(`Conversation for ${anchor.title}`);
  } finally {
    release();
    await Promise.allSettled(forwards);
    await page.unrouteAll({ behavior: "wait" });
    for (const task of tasks) await api.deleteTask(task.id);
  }
}
