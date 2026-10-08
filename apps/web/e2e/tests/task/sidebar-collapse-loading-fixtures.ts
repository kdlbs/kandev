import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { SessionPage } from "../../pages/session-page";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { expectTouchControl } from "../../helpers/control-sizing";
import { heldDisclosure } from "../../helpers/sidebar-disclosure-request";

async function seedRepositories(
  api: ApiClient,
  seed: SeedData,
  directories: string[],
  surface: string,
) {
  const names = [`${surface}-repository-a`, `${surface}-repository-b`];
  const tasks: string[] = [];
  for (const name of names) {
    const dir = mkdtempSync(path.join(path.dirname(seed.repositoryPath), "sidebar-collapse-"));
    directories.push(dir);
    execFileSync("git", ["clone", "--local", seed.repositoryPath, dir], { stdio: "ignore" });
    const repo = await api.createRepository(seed.workspaceId, dir, "main", { name });
    const task = await api.createTask(seed.workspaceId, `Review ${name}`, {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [repo.id],
    });
    await api.archiveTask(task.id);
    tasks.push(task.id);
  }
  const current = await api.createTask(seed.workspaceId, "Open conversation", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const { session_id } = await api.seedTaskSession(current.id, {
    state: "COMPLETED",
    agentProfileId: seed.agentProfileId,
    completedAt: new Date().toISOString(),
  });
  await api.seedSessionMessage(session_id, {
    type: "message",
    content: "Repository collapse preserves this conversation",
  });
  await api.updateTaskState(current.id, "COMPLETED");
  await api.archiveTask(current.id);
  await api.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seed.workspaceId,
      views: [
        {
          id: "collapse-view",
          name: "Repositories",
          filters: [
            { id: "repos", dimension: "repository", op: "in", value: names },
            { id: "archived", dimension: "archived", op: "is", value: true },
          ],
          sort: { key: "title", direction: "asc" },
          group: "repository",
          collapsed_groups: [],
        },
      ],
      active_view_id: "collapse-view",
      draft: null,
    },
  });
  return { names, tasks, current };
}

// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.25
// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.9
export async function exerciseRepositoryCollapse(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  surfaceKind: "desktop" | "picker" | "navigation",
  capture: PrAssetCapture,
) {
  const directories: string[] = [];
  try {
    const { names, tasks, current } = await seedRepositories(api, seed, directories, surfaceKind);
    await page.goto(`/t/${current.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    const mobile = surfaceKind !== "desktop";
    if (surfaceKind === "picker") await page.getByTestId("mobile-task-picker-trigger").tap();
    if (surfaceKind === "navigation") await page.getByTestId("app-nav-trigger").tap();
    let surface = session.sidebar;
    if (surfaceKind === "picker") surface = page.getByRole("dialog", { name: "Tasks" });
    if (surfaceKind === "navigation") surface = page.getByTestId("app-nav-sheet");
    const headers = surface.getByTestId("sidebar-group-header");
    const first = headers.filter({ hasText: names[0] });
    const second = headers.filter({ hasText: names[1] });
    const rowA = surface.locator(`[data-task-row-id="${tasks[0]}"]`);
    const rowB = surface.locator(`[data-task-row-id="${tasks[1]}"]`);
    await expect(rowA).toBeVisible();
    await expect(rowB).toBeVisible();
    if (mobile) {
      await first.scrollIntoViewIfNeeded();
      await expectTouchControl(first);
    }
    await heldDisclosure(page, first, async () => {
      await expect(first).toHaveAttribute("aria-expanded", "false");
      await expect(rowA).toHaveCount(0);
      await expect(rowB).toBeVisible();
      await expect(headers).toHaveCount(2);
      await expect(page).toHaveURL(new RegExp(`/t/${current.id}$`));
      if (mobile) {
        await rowB.scrollIntoViewIfNeeded();
        await expect(rowB).toBeInViewport();
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
      }
      await waitForFiniteAnimations(surface);
      await capture.screenshot(`${surfaceKind}-collapse-refresh`, {
        caption: "Collapsing a repository keeps the other tasks visible while its page refreshes.",
      });
    });
    await expect(surface.getByRole("status").filter({ hasText: "Updating tasks" })).toHaveCount(0);
    await heldDisclosure(page, second, async () => {
      await expect(first).toBeVisible();
      await expect(second).toHaveAttribute("aria-expanded", "false");
      await expect(surface.locator("[data-task-row-id]")).toHaveCount(0);
    });
    await expect(surface.getByRole("status").filter({ hasText: "Updating tasks" })).toHaveCount(0);
    await expect(headers).toHaveCount(2);
    await heldDisclosure(page, first, async () => {
      await expect(first).toHaveAttribute("aria-expanded", "true");
      await expect(headers).toHaveCount(2);
      await expect(rowA).toHaveCount(0);
    });
    await expect(rowA).toBeVisible();
    await expect(rowB).toHaveCount(0);
    if (mobile) {
      await page.keyboard.press("Escape");
      await expect(surface).toBeHidden();
    }
    await expect(
      session.activeChat().getByText("Repository collapse preserves this conversation").last(),
    ).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`/t/${current.id}$`));
  } finally {
    for (const dir of directories) rmSync(dir, { recursive: true, force: true });
  }
}
