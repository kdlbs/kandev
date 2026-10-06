import fs from "node:fs";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import { KanbanPage } from "../../pages/kanban-page";

export async function seedSelectedFilters(api: ApiClient, seed: SeedData, tmpDir: string) {
  for (const suffix of ["A", "B", "C", "D"]) {
    const directory = path.join(tmpDir, `filter-repo-${suffix}`);
    fs.mkdirSync(directory, { recursive: true });
    const env = makeGitEnv(tmpDir);
    execFileSync("git", ["init", "-b", "main"], { cwd: directory, env });
    execFileSync("git", ["commit", "--allow-empty", "-m", "init"], { cwd: directory, env });
    await api.createRepository(seed.workspaceId, directory, "main", {
      name: `Filter repository ${suffix}`,
    });
  }
  const { repositories } = await api.listRepositories(seed.workspaceId);
  const source = repositories.map((repository) => repository.name);
  const selected = source.slice(-2);
  const unselected = source.slice(0, -2);
  const stepIds: string[] = [];
  for (const name of ["Alpha filter", "Beta filter"]) {
    const workflow = await api.createWorkflow(seed.workspaceId, name);
    const step = await api.createWorkflowStep(workflow.id, "Done", 100);
    stepIds.push(step.id);
    await api.seedTask(seed.workspaceId, `${name} task`, {
      workflow_id: workflow.id,
      workflow_step_id: step.id,
    });
  }
  const filters = [
    { id: "repo-filter", dimension: "repository", op: "in", value: [...selected].reverse() },
    { id: "step-filter", dimension: "workflowStep", op: "not_in", value: [...stepIds].reverse() },
  ];
  const savedView = {
    id: "selected-first",
    name: "Selected first",
    filters,
    sort: { key: "state", direction: "asc" },
    group: "none",
    collapsed_groups: [],
  };
  const response = await api.rawRequest("PATCH", "/api/v1/user/settings", {
    sidebar_view_state: {
      workspace_id: seed.workspaceId,
      views: [savedView],
      active_view_id: savedView.id,
      draft: null,
    },
  });
  expect(response.ok).toBe(true);
  await api.saveUserSettings({ workflow_filter_id: "", enable_preview_on_click: false });
  const nav = await api.seedTask(seed.workspaceId, "Filter ordering navigation", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  return {
    source,
    selected,
    unselected,
    stepIds,
    filters,
    taskId: nav.task_id,
    workflowId: seed.workflowId,
    startStepIndex: seed.steps.findIndex((step) => step.id === seed.startStepId),
  };
}

export async function openSelectedFilters(
  page: Page,
  seeded: Awaited<ReturnType<typeof seedSelectedFilters>>,
  mobile: boolean,
) {
  const kanban = new KanbanPage(page);
  await kanban.goto();
  if (mobile) {
    await page.getByTestId("mobile-board-navigator").tap();
    await page.getByTestId(`mobile-workflow-item-${seeded.workflowId}`).tap();
    await page
      .getByTestId("mobile-board-navigator-drawer")
      .getByTestId(`column-tab-${seeded.startStepIndex}`)
      .tap();
  } else {
    await expect(kanban.taskCardByTitle("Alpha filter task")).toBeVisible();
    await expect(kanban.taskCardByTitle("Beta filter task")).toBeVisible();
  }
  await activate(kanban.taskCard(seeded.taskId), mobile);
  await expect(page).toHaveURL(new RegExp(`/t/${seeded.taskId}`));
  await new SessionPage(page).waitForLoad();
  if (mobile) {
    await page.getByTestId("mobile-task-picker-trigger").tap();
    await page.getByRole("dialog", { name: "Tasks" }).getByTestId("sidebar-filter-gear").tap();
    await expect(page.getByTestId("sidebar-filter-drawer")).toBeVisible();
  } else {
    await new SidebarFilterPopoverPage(page).open();
  }
  return page.getByTestId("sidebar-filter-popover");
}

export async function activate(locator: Locator, mobile: boolean) {
  if (mobile) await locator.tap();
  else await locator.click();
}

export async function verifySelectedFilters(
  page: Page,
  editor: Locator,
  seeded: Awaited<ReturnType<typeof seedSelectedFilters>>,
  mobile: boolean,
) {
  const trigger = editor.getByTestId("filter-value-multi").first();
  await activate(trigger, mobile);
  const picker = page.getByTestId("filter-value-multi-popover");
  const rows = picker.getByTestId("filter-value-multi-option");
  await expect(rows).toHaveText([...seeded.selected, ...seeded.unselected]);
  await expect(rows.nth(0)).toHaveAttribute("data-checked", "true");
  await expect(rows.nth(1)).toHaveAttribute("data-checked", "true");
  if (mobile) {
    const rowBox = await rows.nth(0).boundingBox();
    expect(rowBox!.height).toBeGreaterThanOrEqual(44);
  }
  const search = picker.getByRole("combobox");
  await search.fill(seeded.unselected[0]);
  await expect(rows).toHaveText([seeded.unselected[0]]);
  await search.fill("");
  await expect(rows).toHaveText([...seeded.selected, ...seeded.unselected]);
  await activate(rows.filter({ hasText: seeded.selected[0] }), mobile);
  await expect(rows).toHaveText([
    seeded.selected[1],
    ...seeded.source.filter((value) => value !== seeded.selected[1]),
  ]);
  await expect(rows.nth(0)).toHaveAttribute("data-checked", "true");
  await search.fill(seeded.unselected[0]);
  await page.keyboard.press("Escape");
  await expect(picker).toBeHidden();
  await activate(trigger, mobile);
  await expect(rows.nth(0)).toHaveText(seeded.selected[1]);
  await expect(search).toHaveValue("");
  await page.keyboard.press("Escape");
  await expect(picker).toBeHidden();
  if (!mobile) await expect(trigger).toBeFocused();
  await activate(editor.getByTestId("filter-value-multi").nth(1), mobile);
  await expect(rows.nth(0)).toHaveAttribute("data-checked", "true");
  await expect(rows.nth(1)).toHaveAttribute("data-checked", "true");
  await expect(rows.nth(2)).toHaveAttribute("data-checked", "false");
  const context = await rows.evaluateAll((elements) =>
    elements.slice(0, 2).map((element) => ({
      value: element.getAttribute("data-value"),
      heading: element.closest("[cmdk-group]")?.querySelector("[cmdk-group-heading]")?.textContent,
    })),
  );
  expect(context.map((row) => row.heading)).toEqual(["Alpha filter", "Beta filter"]);
  expect(context.map((row) => row.value?.split(" ").at(-1))).toEqual(seeded.stepIds);
  const box = await picker.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.y + box!.height).toBeLessThanOrEqual(page.viewportSize()!.height);
  expect(
    await picker.getByRole("listbox").evaluate((element) => getComputedStyle(element).overflowY),
  ).toBe("auto");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}
