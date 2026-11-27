import fs from "node:fs";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import { repositorySlug } from "../../../lib/repository-slug";

export async function seedRepositoryNone(api: ApiClient, seed: SeedData, tmpDir: string) {
  const directory = path.join(tmpDir, "none-filter-repo");
  fs.mkdirSync(directory, { recursive: true });
  const env = makeGitEnv(tmpDir);
  execFileSync("git", ["init", "-b", "main"], { cwd: directory, env });
  execFileSync("git", ["commit", "--allow-empty", "-m", "init"], { cwd: directory, env });
  const repoB = await api.createRepository(seed.workspaceId, directory, "main", {
    name: "Other filter repository",
  });
  const { repositories } = await api.listRepositories(seed.workspaceId);
  const repository = repositories.find((repo) => repo.id === seed.repositoryId)!;
  const options = { workflow_id: seed.workflowId, workflow_step_id: seed.startStepId };
  const none = await api.createTask(seed.workspaceId, "Filter none: no repository", {
    ...options,
    repository_ids: [],
  });
  expect((await api.getTask(none.id)).repositories ?? []).toEqual([]);
  const first = await api.createTask(seed.workspaceId, "Filter none: repository A", {
    ...options,
    repository_ids: [seed.repositoryId],
  });
  const second = await api.createTask(seed.workspaceId, "Filter none: repository B", {
    ...options,
    repository_ids: [repoB.id],
  });
  const excluded = await api.createTask(seed.workspaceId, "Excluded by title", {
    ...options,
    repository_ids: [],
  });
  const filters = [
    { id: "repository", dimension: "repository", op: "in", value: [repositorySlug(repository)] },
    { id: "title", dimension: "titleMatch", op: "matches", value: "Filter none:" },
  ];
  const saved = await api.rawRequest("PATCH", "/api/v1/user/settings", {
    sidebar_view_state: {
      workspace_id: seed.workspaceId,
      views: [
        {
          id: "none-filter",
          name: "Repository choices",
          filters,
          sort: { key: "title", direction: "asc" },
          group: "none",
          collapsed_groups: [],
        },
      ],
      active_view_id: "none-filter",
      draft: null,
    },
  });
  expect(saved.ok).toBe(true);
  await api.saveUserSettings({ workflow_filter_id: "", enable_preview_on_click: false });
  const otherWorkspace = await api.createWorkspace("Other repository filter workspace");
  await api.createWorkflow(otherWorkspace.id, "Other workflow", "simple");
  return {
    none,
    first,
    second,
    excluded,
    repository: repositorySlug(repository),
    workspaceId: seed.workspaceId,
    otherWorkspaceId: otherWorkspace.id,
  };
}

type Seeded = Awaited<ReturnType<typeof seedRepositoryNone>>;

async function activate(control: Locator, mobile: boolean) {
  if (mobile) await control.tap();
  else await control.click();
}

export async function openRepositoryNone(page: Page, seeded: Seeded, mobile: boolean) {
  await page.goto(`/t/${seeded.none.id}`);
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

async function expectTasks(page: Page, seeded: Seeded, mobile: boolean, ids: string[]) {
  const surface = mobile
    ? page.getByRole("dialog", { name: "Tasks" })
    : page.getByTestId("app-sidebar-content");
  for (const task of [seeded.none, seeded.first, seeded.second, seeded.excluded]) {
    const row = surface.locator(`[data-task-row-id="${task.id}"]`);
    if (ids.includes(task.id)) await expect(row).toBeVisible();
    else await expect(row).toHaveCount(0);
  }
}

async function expectDraft(api: ApiClient, seeded: Seeded, values: string[]) {
  await expect
    .poll(async () => {
      const { settings } = await api.getUserSettings();
      return settings.sidebar_views_by_workspace[seeded.workspaceId].draft?.filters[0].value;
    })
    .toEqual(values);
}

async function selectRepositoryNone(page: Page, editor: Locator, mobile: boolean) {
  const trigger = editor.getByTestId("filter-value-multi");
  await activate(trigger, mobile);
  const picker = page.getByTestId("filter-value-multi-popover");
  const none = picker.getByRole("option", { name: "None", exact: true });
  await expect(none).toBeVisible();
  if (mobile) {
    await waitForFiniteAnimations(picker);
    expect((await none.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    const box = (await picker.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    // Mobile scaling can place an edge between CSS pixels.
    expect(Math.round(box.x + box.width)).toBeLessThanOrEqual(page.viewportSize()!.width);
    expect(box.y).toBeGreaterThanOrEqual(0);
    expect(Math.round(box.y + box.height)).toBeLessThanOrEqual(page.viewportSize()!.height);
  }
  const search = picker.getByRole("combobox");
  await search.fill("None");
  if (mobile) await none.tap();
  else {
    await expect(none).toHaveAttribute("data-selected", "true");
    await search.press("Enter");
  }
  await search.fill("");
  await expect(none).toHaveAttribute("data-checked", "true");
  await page.keyboard.press("Escape");
  await expect(picker).toBeHidden();
  if (!mobile) await expect(trigger).toBeFocused();
}

export async function verifyRepositoryNone(
  page: Page,
  api: ApiClient,
  seeded: Seeded,
  mobile: boolean,
) {
  const editor = await openRepositoryNone(page, seeded, mobile);
  await selectRepositoryNone(page, editor, mobile);
  await activate(editor.getByTestId("view-save-button"), mobile);
  await expect
    .poll(async () => {
      const { settings } = await api.getUserSettings();
      return settings.sidebar_views_by_workspace[seeded.workspaceId].views[0].filters[0].value;
    })
    .toEqual([seeded.repository, "undefined"]);
  await page.keyboard.press("Escape");
  await expect(editor).toBeHidden();
  await expectTasks(page, seeded, mobile, [seeded.none.id, seeded.first.id]);

  const reopened = await openRepositoryNone(page, seeded, mobile);
  const trigger = reopened.getByTestId("filter-value-multi");
  await activate(trigger, mobile);
  const picker = page.getByTestId("filter-value-multi-popover");
  await expect(picker.getByRole("option", { name: "None", exact: true })).toHaveAttribute(
    "data-checked",
    "true",
  );
  await activate(picker.getByRole("option", { name: seeded.repository, exact: true }), mobile);
  await page.keyboard.press("Escape");
  await expect(picker).toBeHidden();
  await expect(trigger).toHaveText("None");
  await page.keyboard.press("Escape");
  await expect(reopened).toBeHidden();
  await expectTasks(page, seeded, mobile, [seeded.none.id]);
  await expectDraft(api, seeded, ["undefined"]);

  await page.goto(`/?workspaceId=${seeded.otherWorkspaceId}`);
  await expect(
    page.getByTestId(mobile ? "mobile-board-navigator" : "display-button"),
  ).toBeVisible();
  const returned = await openRepositoryNone(page, seeded, mobile);
  await expect(returned.getByTestId("filter-value-multi")).toHaveText("None");
  await activate(returned.getByTestId("view-discard-button"), mobile);
  await activate(returned.getByTestId("filter-value-multi"), mobile);
  const restoredPicker = page.getByTestId("filter-value-multi-popover");
  const restoredNone = restoredPicker.getByRole("option", { name: "None", exact: true });
  await expect(restoredNone).toHaveAttribute("data-checked", "true");
  await activate(restoredNone, mobile);
  await expect(restoredNone).toHaveAttribute("data-checked", "false");
  await expectDraft(api, seeded, [seeded.repository]);
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("filter-value-multi-popover")).toBeHidden();
  await page.keyboard.press("Escape");
  await expectTasks(page, seeded, mobile, [seeded.first.id]);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}
