import { expect, type Page, type Route } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { seedActionThreads, ThreadActionsPage } from "./threads-task-actions-helpers";

// @covers AC-TASKS-THREADS-ACTIONS-002.5, AC-TASKS-THREADS-ACTIONS-003.4
export async function failedActionOutcomes(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const { a, b, destination } = await seedActionThreads(api, seed);
  const before = await api.getTask(a.id);
  const ui = new ThreadActionsPage(page, mobile);
  await api.saveUserSettings({ confirm_task_archive: true });
  let failure: { path: string; method: string } | null = null;
  let intercepted = 0;
  await page.route("**/api/v1/tasks/**", async (route) => {
    const request = route.request();
    if (
      failure &&
      new URL(request.url()).pathname === failure.path &&
      request.method() === failure.method
    ) {
      failure = null;
      intercepted++;
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "Task actions offline" }),
      });
    } else await route.continue();
  });
  await page.goto(`/threads?workspace=${seed.workspaceId}`);
  const cases = [
    {
      action: "Priority",
      path: `/api/v1/tasks/${a.id}`,
      method: "PATCH",
      error: "Failed to update task",
      choose: async () => {
        await ui.nested("Priority");
        await ui.pick("Critical");
      },
    },
    {
      action: "Move to",
      path: "/api/v1/tasks/bulk-move",
      method: "POST",
      error: "Failed to move task",
      choose: async () => {
        await ui.nested("Move to");
        await ui.pick("Task actions triage");
      },
    },
    {
      action: "Send to workflow",
      path: "/api/v1/tasks/bulk-move",
      method: "POST",
      error: "Failed to move task",
      choose: async () => {
        await ui.nested("Send to workflow");
        await ui.nested(destination.name);
        await ui.pick("Incoming");
      },
    },
    {
      action: "Archive",
      path: `/api/v1/tasks/${a.id}/archive`,
      method: "POST",
      error: "Failed to archive task",
      choose: async () => {
        await ui.pick("Archive");
        await ui.press(page.getByTestId("archive-task-confirm"));
      },
    },
    {
      action: "Delete",
      path: `/api/v1/tasks/${a.id}`,
      method: "DELETE",
      error: "Failed to delete task",
      choose: async () => {
        await ui.pick("Delete");
        await ui.confirmDelete();
      },
    },
  ];
  for (const [index, scenario] of cases.entries()) {
    failure = scenario;
    await ui.open(a.id);
    await scenario.choose();
    await expect
      .poll(() => intercepted, { message: `${scenario.action} reached its task API` })
      .toBe(index + 1);
    await expect(page.getByText(scenario.error, { exact: true }).last()).toBeVisible();
    await expect(ui.column(a.id)).toBeVisible();
    expect(await api.getTask(a.id)).toMatchObject({
      priority: before.priority,
      workflow_step_id: before.workflow_step_id,
    });
    await expect(ui.column(b.id)).toHaveCount(1);
  }
  await ui.open(a.id);
  await ui.nested("Link");
  await ui.pick("GitHub Issue");
  await ui.press(page.getByTestId("task-github-issue-submit"));
  await expect(page.getByTestId("task-github-issue-error")).toBeVisible();
  await ui.press(page.getByRole("button", { name: "Cancel", exact: true }));
  await expect(ui.trigger(a.id)).toBeFocused();
  await ui.open(a.id);
  await ui.nested("Priority");
  await ui.pick("Low");
  await expect.poll(async () => (await api.getTask(a.id)).priority).toBe("low");
}

// @covers AC-TASKS-THREADS-ACTIONS-002.2, AC-TASKS-THREADS-ACTIONS-002.5, AC-TASKS-THREADS-ACTIONS-002.6
export async function lateArchiveOutcome(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const { a, b } = await seedActionThreads(api, seed);
  const ui = new ThreadActionsPage(page, mobile);
  await api.saveUserSettings({ confirm_task_archive: false });
  let pending: Route | null = null;
  await page.route(`**/api/v1/tasks/${a.id}/archive`, (route) => {
    pending = route;
  });
  await page.goto(`/threads?workspace=${seed.workspaceId}`);
  try {
    await ui.open(a.id);
    await ui.pick("Archive");
    await expect.poll(() => pending !== null).toBe(true);
    await expect(page.getByTestId("archive-task-confirm")).toHaveCount(0);
    await expect(ui.column(a.id)).toHaveCount(0);
    await ui.open(b.id);
    await expect(ui.choice("Delete")).toBeDisabled();
    await pending!.continue();
    pending = null;
    await expect(ui.column(a.id)).toHaveCount(0);
    await expect(ui.choice("Delete")).toBeEnabled();
    await ui.nested("Priority");
    await ui.pick("Critical");
    await expect.poll(async () => (await api.getTask(b.id)).priority).toBe("critical");
    expect((await api.getTask(a.id)).priority).not.toBe("critical");
    await expect(ui.trigger(b.id)).toBeFocused();
  } finally {
    if (pending) await (pending as Route).abort();
    await page.unroute(`**/api/v1/tasks/${a.id}/archive`);
    await api.saveUserSettings({ confirm_task_archive: true });
  }
}

// @covers AC-TASKS-THREADS-ACTIONS-002.3, AC-TASKS-THREADS-ACTIONS-003.1, AC-TASKS-THREADS-ACTIONS-003.2
export async function filteredArchiveOutcome(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const { a, b } = await seedActionThreads(api, seed);
  for (const task of [a, b])
    expect(
      (await api.rawRequest("PATCH", `/api/v1/tasks/${task.id}`, { priority: "medium" })).ok,
    ).toBe(true);
  const view = {
    id: "actions-normal",
    name: "Normal priority",
    task_scope: { mode: "all", task_ids: [] },
    filters: [{ id: "normal", dimension: "priority", op: "is", value: "medium" }],
    sort: { key: "title", direction: "asc" },
    max_columns: null,
  };
  expect(
    (
      await api.rawRequest("PATCH", "/api/v1/user/settings", {
        confirm_task_archive: true,
        thread_views: [view],
        thread_active_view_id: view.id,
        thread_view_draft: null,
      })
    ).ok,
  ).toBe(true);
  const ui = new ThreadActionsPage(page, mobile);
  let classification: Route | null = null;
  await page.route(`**/api/v1/tasks/${a.id}/subtask-count`, (route) => {
    classification = route;
  });
  await page.goto(`/threads?workspace=${seed.workspaceId}`);
  await ui.open(a.id);
  await ui.pick("Archive");
  await expect.poll(() => classification !== null).toBe(true);
  expect((await api.rawRequest("PATCH", `/api/v1/tasks/${a.id}`, { priority: "low" })).ok).toBe(
    true,
  );
  await expect(ui.column(a.id)).toHaveCount(0);
  await classification!.continue();
  await ui.press(page.getByTestId("archive-task-confirm"));
  await expect.poll(async () => (await api.getTask(a.id)).archived_at).toBeTruthy();
  await expect(ui.trigger(b.id)).toBeFocused();
  expect((await api.getTask(b.id)).archived_at).toBeFalsy();
  await expect(page).toHaveURL(new RegExp(`/threads\\?workspace=${seed.workspaceId}`));
}
