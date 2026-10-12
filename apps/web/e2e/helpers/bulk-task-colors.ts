import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";

export async function seedColorTasks(api: ApiClient, seed: SeedData) {
  const tasks = [];
  for (const title of ["Bulk color A", "Bulk color B", "Unselected color control"]) {
    tasks.push(
      await api.createTask(seed.workspaceId, title, {
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
      }),
    );
  }
  return tasks;
}

export async function expectStoredColors(api: ApiClient, ids: string[], color: string | null) {
  await expect
    .poll(async () => {
      const { settings } = await api.getUserSettings();
      const colors = settings.sidebar_task_colors as Record<string, string | null>;
      return ids.map((id) => colors[id] ?? null);
    })
    .toEqual(ids.map(() => color));
}

export async function chooseSidebarColor(page: Page, label: string) {
  await page.getByRole("menuitem", { name: "Color", exact: true }).hover();
  const role = label === "None" ? "menuitem" : "menuitemradio";
  await page.getByRole(role, { name: label, exact: true }).click();
}

export async function expectNoHorizontalOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}
