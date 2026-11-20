import { execFileSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Locator, Page, TestInfo } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { waitForFiniteAnimations } from "../../helpers/animations";

async function activate(control: Locator, mobile: boolean) {
  await expect(control).toHaveCount(1);
  if (mobile) await control.tap();
  else await control.click();
}

async function openLink(page: Page, taskId: string, mobile: boolean) {
  let surface: Locator;
  if (mobile) {
    surface = page.getByRole("dialog", { name: "Tasks", exact: true });
    if (!(await surface.isVisible())) await page.getByTestId("mobile-task-picker-trigger").tap();
  } else surface = page.getByTestId("task-sidebar");
  const row = surface.locator(`[data-task-row-id="${taskId}"]`);
  await expect(row).toHaveCount(1);
  if (!mobile) await row.hover();
  await activate(row.getByRole("button", { name: "Task actions", exact: true }), mobile);
  const link = page.getByRole("menuitem", { name: "Link", exact: true });
  if (mobile) await link.tap();
  else await link.hover();
  await activate(page.getByRole("menuitem", { name: "Jira Ticket", exact: true }), mobile);
  const dialog = page.getByRole("dialog", { name: "Link Jira ticket", exact: true });
  await expect(dialog).toBeVisible();
  await waitForFiniteAnimations(dialog);
  return dialog;
}

function repositorySnapshot(repositoryPath: string) {
  return {
    head: execFileSync("git", ["-C", repositoryPath, "rev-parse", "HEAD"], { encoding: "utf8" }),
    status: execFileSync("git", ["-C", repositoryPath, "status", "--porcelain"], {
      encoding: "utf8",
    }),
  };
}

// @covers AC-TASKS-TASK-ACTIONS-MENU-004.7
export async function preserveExternalLinkDraft(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  info: TestInfo,
  { mobile, sameTask }: { mobile: boolean; sameTask: boolean },
) {
  const seedBefore = repositorySnapshot(seed.repositoryPath);
  const workspace = await api.createWorkspace(`External link ownership ${info.testId}`);
  let release = () => {};
  try {
    const workflow = await api.createWorkflow(workspace.id, "Link ownership", "simple");
    const { steps } = await api.listWorkflowSteps(workflow.id);
    expect(steps.length).toBeGreaterThan(0);
    await api.setJiraConfig({
      workspaceId: workspace.id,
      siteUrl: "https://fixture.atlassian.net",
      email: "fixture@example.test",
      secret: "fixture-only-token",
    });
    await api.waitForIntegrationAuthHealthy("jira", { workspaceId: workspace.id });
    await api.mockJiraAddTickets(
      ["APP-1", "APP-2"].map((key) => ({
        key,
        summary: key,
        description: "Fixture issue",
        url: `https://fixture.atlassian.net/browse/${key}`,
      })),
    );
    expect(seed.repositoryPath.startsWith(join(tmpdir(), "kandev-e2e-"))).toBe(true);
    const repository = await api.createRepository(workspace.id, seed.repositoryPath);
    const options = {
      workflow_id: workflow.id,
      workflow_step_id: steps[0].id,
      repository_ids: [repository.id],
      agent_profile_id: seed.agentProfileId,
      executor_profile_id: seed.worktreeExecutorProfileId,
    };
    const a = await api.createTask(workspace.id, "Original link target", options);
    const b = await api.createTask(workspace.id, "Later link target", options);
    let admitted!: () => void;
    const admission = new Promise<void>((resolve) => {
      admitted = resolve;
    });
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const patches: unknown[] = [];
    let first = true;
    await page.route(`**/api/v1/tasks/${a.id}`, async (route) => {
      if (route.request().method() !== "PATCH") return route.continue();
      patches.push(route.request().postDataJSON());
      if (first) {
        first = false;
        admitted();
        await held;
      }
      const response = await route.fetch();
      await route.fulfill({ response });
    });
    await page.goto(`/t/${a.id}`);
    await new SessionPage(page).waitForLoad();
    await expect(page.getByText("Couldn't start a session", { exact: true })).toHaveCount(0);
    const original = await openLink(page, a.id, mobile);
    await original.getByTestId("task-external-link-input").fill("app-1");
    await activate(original.getByTestId("task-external-link-submit"), mobile);
    await admission;
    expect(patches).toEqual([{ title: "APP-1: Original link target" }]);
    expect((await api.getTask(a.id)).title).toBe("Original link target");
    if (mobile) await original.getByRole("button", { name: "Close", exact: true }).tap();
    else await page.keyboard.press("Escape");
    await expect(original).toBeHidden();
    const later = await openLink(page, sameTask ? a.id : b.id, mobile);
    const input = later.getByTestId("task-external-link-input");
    await expect(input).toBeEnabled();
    await activate(input, mobile);
    await input.fill("  app-2  ");
    await expect(input).toBeFocused();
    const settled = page.waitForResponse(
      (res) => res.url().endsWith(`/api/v1/tasks/${a.id}`) && res.request().method() === "PATCH",
    );
    release();
    const response = await settled;
    expect(response.status()).toBe(200);
    await response.finished();
    await expect(page.getByText("Jira ticket linked", { exact: true })).toBeVisible();
    await expect(input).toHaveValue("  app-2  ");
    await expect(input).toBeEnabled();
    await expect(input).toBeFocused();
    await expect(later.getByTestId("task-external-link-error")).toHaveCount(0);
    expect((await api.getTask(a.id)).title).toBe("APP-1: Original link target");
    expect((await api.getTask(b.id)).title).toBe("Later link target");
    await info.attach("retained-external-link-draft", {
      body: await page.screenshot({ path: info.outputPath("retained-external-link-draft.png") }),
      contentType: "image/png",
    });
    if (sameTask) {
      await activate(later.getByRole("button", { name: "Cancel", exact: true }), mobile);
      await expect(later).toBeHidden();
      expect(patches).toHaveLength(1);
      expect((await api.getTask(a.id)).title).toBe("APP-1: Original link target");
    } else {
      await activate(later.getByTestId("task-external-link-submit"), mobile);
      await expect(later).toBeHidden();
      await expect
        .poll(async () => (await api.getTask(b.id)).title)
        .toBe("APP-2: Later link target");
    }
    await page.reload();
    await new SessionPage(page).waitForLoad();
    expect((await api.getTask(a.id)).title).toBe("APP-1: Original link target");
    expect((await api.getTask(b.id)).title).toBe(
      sameTask ? "Later link target" : "APP-2: Later link target",
    );
  } finally {
    release();
    await page.unrouteAll({ behavior: "wait" });
    await page.goto("/");
    await api.deleteWorkspace(workspace.id, workspace.name);
    expect(repositorySnapshot(seed.repositoryPath)).toEqual(seedBefore);
  }
}
