import { expect, type Page } from "@playwright/test";
import type { StoreApi } from "zustand";
import type { AppState } from "../../../lib/state/store";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { readBootCapture } from "./journey-boot-loading-helpers";

export function recordWorkflowSnapshots(page: Page) {
  const requests: string[] = [];
  page.on("request", (request) => {
    const match = new URL(request.url()).pathname.match(/\/api\/v1\/workflows\/([^/]+)\/snapshot$/);
    if (match) requests.push(match[1]);
  });
  return requests;
}

export async function seedDemandBoards(api: ApiClient, seed: SeedData) {
  const boards = [];
  for (let index = 0; index < 20; index++) {
    const workflow = await api.createWorkflow(seed.workspaceId, `Review demand ${index}`, "simple");
    const { steps } = await api.listWorkflowSteps(workflow.id);
    const task = await api.createTask(seed.workspaceId, `Demand task ${index}`, {
      workflow_id: workflow.id,
      workflow_step_id: steps[0].id,
    });
    boards.push({ ...workflow, task });
  }
  await api.saveUserSettings({ workspace_id: seed.workspaceId, workflow_filter_id: "" });
  return boards;
}

export async function expectFirstBoardSeed(page: Page) {
  const boot = await readBootCapture(page);
  const ids = Object.keys(boot.initialState.kanbanMulti?.snapshots ?? {});
  expect(ids).toHaveLength(1);
  return ids[0];
}

export async function assertBoundedClientNavigation(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  await page.addInitScript(() => {
    (window as Window & { __KANDEV_E2E_EXPOSE_STORE__?: boolean }).__KANDEV_E2E_EXPOSE_STORE__ =
      true;
  });
  const [source, destination] = await seedBoundedNavigation(api, seed);
  const snapshots = recordWorkflowSnapshots(page);
  const pageSizes: number[] = [];
  const pageResponses: Promise<void>[] = [];
  page.on("response", (response) => {
    if (new URL(response.url()).pathname.endsWith("/sidebar/query") && response.ok()) {
      pageResponses.push(
        response.json().then((body: { entries?: Array<{ task_id?: string }> }) => {
          pageSizes.push(body.entries?.filter((entry) => entry.task_id).length ?? 0);
        }),
      );
    }
  });
  await page.goto(`/t/${source.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(session.activeChat()).toContainText(`History for ${source.title}`);
  const boot = await readBootCapture(page);
  expect(
    boot.routeData?.taskDetail?.sidebarTaskPage?.entries?.filter((entry) => entry.task_id).length,
  ).toBeLessThanOrEqual(100);
  if (mobile) await page.getByTestId("mobile-task-picker-trigger").tap();
  const surface = mobile
    ? page.getByRole("dialog", { name: "Tasks", exact: true })
    : session.sidebar;
  const row = surface.locator(`[data-task-row-id="${destination.id}"]`);
  await expect(row).toBeVisible();
  const enrichmentRead = page.waitForResponse((response) => {
    const path = new URL(response.url()).pathname;
    return (
      response.ok() &&
      (path === `/api/v1/workflows/${seed.workflowId}/workflow/steps` ||
        path === `/api/v1/workflows/${seed.workflowId}/snapshot`)
    );
  });
  if (mobile) await row.tap();
  else await row.click();
  await (await enrichmentRead).finished();
  await expect(page).toHaveURL(new RegExp(`/t/${destination.id}(?:\\?|$)`));
  await expect(session.activeChat()).toContainText(`History for ${destination.title}`);
  expect((await readBootCapture(page)).routeData?.taskDetail?.taskId).toBe(source.id);
  expect(snapshots).toEqual([]);
  await Promise.all(pageResponses);
  expect(pageSizes.every((size) => size <= 100)).toBe(true);
  const state = await page.evaluate((workflowId) => {
    const store = (window as Window & { __KANDEV_E2E_STORE__?: StoreApi<AppState> })
      .__KANDEV_E2E_STORE__;
    if (!store) throw new Error("Missing route store");
    const state = store.getState();
    return {
      tasks: Object.keys(state.taskOverview.byId).length,
      coverage: state.kanban.taskCoverage?.complete,
      completeBoard: state.kanbanMulti.snapshots[workflowId]?.taskCoverage?.complete ?? false,
    };
  }, seed.workflowId);
  expect(state.tasks).toBeLessThanOrEqual(102);
  expect(state.coverage).not.toBe(true);
  expect(state.completeBoard).toBe(false);
  return { snapshots, pageSizes, state };
}

async function seedBoundedNavigation(api: ApiClient, seed: SeedData) {
  for (let index = 0; index < 205; index++) {
    await api.createTask(seed.workspaceId, `Unrelated navigation ${index}`, {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
    });
  }
  const targets = [];
  for (const title of ["Bounded source", "Bounded destination"]) {
    const task = await api.createTask(seed.workspaceId, title, {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
    });
    const { session_id } = await api.seedTaskSession(task.id, {
      state: "COMPLETED",
      agentProfileId: seed.agentProfileId,
      completedAt: new Date().toISOString(),
    });
    await api.seedSessionMessage(session_id, { type: "message", content: `History for ${title}` });
    targets.push(task);
  }
  const saved = await api.rawRequest("PATCH", "/api/v1/user/settings", {
    sidebar_view_state: {
      workspace_id: seed.workspaceId,
      active_view_id: "review-bounded",
      draft: null,
      views: [
        {
          id: "review-bounded",
          name: "Review bounded",
          filters: [],
          sort: { key: "updatedAt", direction: "desc" },
          group: "none",
          collapsed_groups: [],
        },
      ],
    },
  });
  expect(saved.ok).toBe(true);
  return targets;
}
