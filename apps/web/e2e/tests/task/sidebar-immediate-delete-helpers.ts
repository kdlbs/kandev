import { expect, type Page, type Route } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

// @covers AC-TASKS-REMOVAL-NAVIGATION-005.1, AC-TASKS-REMOVAL-NAVIGATION-005.2
export async function checkImmediateDelete(options: {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  mobile: boolean;
}) {
  const { page, api, seed, mobile } = options;
  const taskOptions = { workflow_id: seed.workflowId, workflow_step_id: seed.startStepId };
  const nav = await api.seedTask(seed.workspaceId, "Keep selected", taskOptions);
  const target = await api.seedTask(seed.workspaceId, "Delete target", taskOptions);
  const path = `**/api/v1/tasks/${target.task_id}*`;
  let pending: Route | null = null;
  let deleteRequests = 0;
  const heldRefreshes: Route[] = [];
  const refreshPath = `**/api/v1/workspaces/${seed.workspaceId}/sidebar/query`;
  const holdRefresh = (route: Route) => {
    heldRefreshes.push(route);
  };
  const handler = async (route: Route) => {
    if (route.request().method() !== "DELETE") return route.continue();
    deleteRequests += 1;
    pending = route;
  };
  await page.route(path, handler);
  const session = new SessionPage(page);
  const sheet = page.getByRole("dialog", { name: "Tasks", exact: true });
  const rows = () => (mobile ? sheet : session.sidebar).getByTestId("sidebar-task-item");
  const targetRow = () => rows().filter({ hasText: "Delete target" });
  const openPicker = async () => {
    if (mobile && !(await sheet.isVisible()))
      await page.getByTestId("mobile-task-picker-trigger").tap();
  };
  const openDelete = async () => {
    await openPicker();
    await expect(targetRow()).toHaveCount(1);
    if (mobile) {
      await targetRow().getByRole("button", { name: "Task actions" }).tap();
      await page.getByRole("menuitem", { name: "Delete", exact: true }).tap();
      await expect(sheet).toBeHidden();
    } else {
      await targetRow().click({ button: "right" });
      await page.getByRole("menuitem", { name: "Delete", exact: true }).click();
    }
  };
  const confirmDelete = async () => {
    const button = page
      .getByRole("alertdialog")
      .getByRole("button", { name: "Delete", exact: true });
    await expect(button).toBeEnabled();
    if (mobile) await button.tap();
    else await button.click();
    await expect.poll(() => pending !== null).toBe(true);
    await expect(page.getByRole("alertdialog")).toBeHidden();
    if (mobile) await page.getByTestId("mobile-task-picker-trigger").tap();
    await expect(targetRow()).toHaveAttribute("aria-busy", "true");
    await expect(targetRow()).toHaveAttribute("aria-disabled", "true");
    await expect(targetRow()).toHaveClass(/opacity-60/);
    await expect(targetRow().getByTestId("task-state-removal-pending")).toBeVisible();
    await expect(targetRow()).toBeInViewport({ ratio: 1 });
    await expect(rows().filter({ hasText: "Keep selected" })).not.toHaveAttribute("aria-busy");
    await expect(page).toHaveURL(new RegExp(`/t/${nav.task_id}$`));
  };
  try {
    await page.goto(`/t/${nav.task_id}`);
    await session.waitForLoad();
    await openDelete();
    await page
      .getByRole("alertdialog")
      .getByRole("button", { name: "Cancel", exact: true })
      .click();
    await openPicker();
    await dwell(page, 150, "negative-assertion", "Cancelling delete must not issue a mutation");
    expect(deleteRequests).toBe(0);
    await expect(targetRow()).not.toHaveAttribute("aria-busy");

    await openDelete();
    await confirmDelete();
    await page.screenshot({
      animations: "disabled",
      path: `test-results/delete-pending-${mobile ? "phone" : "desktop"}.png`,
    });
    await pending!.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "Delete unavailable" }),
    });
    pending = null;
    await expect(targetRow()).not.toHaveAttribute("aria-busy");
    await expect(targetRow().getByTestId("task-state-removal-pending")).toHaveCount(0);
    await expect(page).toHaveURL(new RegExp(`/t/${nav.task_id}$`));

    await openDelete();
    await confirmDelete();
    await page.route(refreshPath, holdRefresh);
    await pending!.continue();
    pending = null;
    await expect(targetRow()).toHaveCount(0);
    await expect.poll(() => heldRefreshes.length).toBeGreaterThan(0);
    await expect(page).toHaveURL(new RegExp(`/t/${nav.task_id}$`));
  } finally {
    if (!page.isClosed()) {
      if (pending) await (pending as Route).abort();
      await page.unroute(path, handler);
      await page.unroute(refreshPath, holdRefresh);
    }
  }
}
