import { expect, type Page, type WebSocketRoute } from "@playwright/test";
import type { StoreApi } from "zustand";
import type { AppState } from "../../../lib/state/store";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { anchorTask, surface, waitForCoverage } from "./sidebar-shared-task-state-fixtures";

function gate() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { promise, release };
}

async function contextFixtures(api: ApiClient, seed: SeedData) {
  await api.saveUserSettings({ enable_preview_on_click: false });
  const other = await api.createWorkspace("Shared context B");
  const { workflows } = await api.listWorkflows(other.id);
  const workflow =
    workflows.find((workflow) => !workflow.hidden) ??
    (await api.createWorkflow(other.id, "Shared workflow B", "simple"));
  const { steps } = await api.listWorkflowSteps(workflow.id);
  const step = steps.find((item) => item.is_start_step) ?? steps[0];
  const seeds = [
    seed,
    { ...seed, workspaceId: other.id, workflowId: workflow.id, startStepId: step.id },
  ];
  const contexts = [];
  for (const item of seeds) contexts.push({ seed: item, anchor: await anchorTask(api, item) });
  return contexts;
}

async function switchSurfaceWorkspace(
  page: Page,
  mobile: boolean,
  context: Awaited<ReturnType<typeof contextFixtures>>[number],
) {
  if (mobile) {
    await page
      .getByRole("dialog", { name: "Tasks", exact: true })
      .getByTitle("Switch workspace")
      .tap();
    const name = await page.evaluate(
      (id) =>
        (window as Window & { __KANDEV_E2E_STORE__: StoreApi<AppState> }).__KANDEV_E2E_STORE__
          .getState()
          .workspaces.items.find((item) => item.id === id)!.name,
      context.seed.workspaceId,
    );
    await page.getByRole("menuitem", { name, exact: true }).tap();
  } else {
    await page.getByTestId("sidebar-workspace-trigger").click();
    await page.getByTestId(`sidebar-workspace-item-${context.seed.workspaceId}`).click();
    await page.getByTestId(`task-card-${context.anchor.id}`).click();
  }
  await expect(page).toHaveURL(new RegExp(`/t/${context.anchor.id}$`));
  return surface(page, mobile);
}

async function holdRecovery(page: Page, workspaceId: string, workflowId: string) {
  const held = gate(),
    captured = gate();
  const queryPattern = `**/workspaces/${workspaceId}/sidebar/query`;
  const snapshotPattern = `**/workflows/${workflowId}/snapshot*`;
  await page.route(queryPattern, async (route) => {
    const response = await route.fetch();
    captured.release();
    await held.promise;
    await route.fulfill({ response });
  });
  await page.route(snapshotPattern, async (route) => {
    const response = await route.fetch();
    await held.promise;
    await route.fulfill({ response });
  });
  return {
    captured: captured.promise,
    async release() {
      held.release();
      await page.unroute(queryPattern);
      await page.unroute(snapshotPattern);
    },
  };
}

export async function exerciseSharedContextRecovery(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const contexts = await contextFixtures(api, seed);
  let socket: WebSocketRoute;
  await page.routeWebSocket(/\/ws(?:\?.*)?$/, (route) => {
    socket = route;
    route.connectToServer();
  });
  await page.goto(`/?workspaceId=${seed.workspaceId}`);
  await waitForCoverage(page, seed.workspaceId);
  await page.getByTestId(`task-card-${contexts[0].anchor.id}`).click();
  let currentSurface = await surface(page, mobile);
  for (let index = 0; index < 2; index++) {
    const before = contexts[index % 2],
      after = contexts[(index + 1) % 2];
    await expect(
      currentSurface.rows.locator(`[data-task-row-id="${before.anchor.id}"]`),
    ).toBeVisible();
    const held = await holdRecovery(page, before.seed.workspaceId, before.seed.workflowId);
    try {
      await socket!.close({ code: 1012, reason: "Sidebar reconnect regression" });
      await held.captured;
      currentSurface = await switchSurfaceWorkspace(page, mobile, after);
      await expect(
        currentSurface.rows.locator(`[data-task-row-id="${after.anchor.id}"]`),
      ).toBeVisible();
      await held.release();
      await expect(
        currentSurface.rows.locator(`[data-task-row-id="${before.anchor.id}"]`),
      ).toHaveCount(0);
      await expect
        .poll(() =>
          page.evaluate(
            (workspaceId) =>
              Object.values(
                (
                  window as Window & { __KANDEV_E2E_STORE__: StoreApi<AppState> }
                ).__KANDEV_E2E_STORE__.getState().taskOverview.byId,
              ).every((task) => task.workspaceId === workspaceId),
            after.seed.workspaceId,
          ),
        )
        .toBe(true);
    } finally {
      await held.release();
    }
  }
  const active = contexts[0];
  await socket!.close({ code: 1012, reason: "Sidebar authoritative recovery" });
  await api.updateTaskTitle(active.anchor.id, "Recovered conversation anchor");
  await waitForCoverage(page, active.seed.workspaceId);
  await expect(
    currentSurface.rows.locator(`[data-task-row-id="${active.anchor.id}"]`),
  ).toContainText("Recovered conversation anchor");
  await expect(page).toHaveURL(new RegExp(`/t/${active.anchor.id}$`));
}
