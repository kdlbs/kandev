import type { Locator, Page, TestInfo } from "@playwright/test";
import { expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import type { BackendContext } from "../../fixtures/backend";
import { withIsolatedRenameFixture } from "./sidebar-task-rename-fixture";

type Scenario = {
  entry: "sidebar" | "commands";
  sameTask: boolean;
  touch?: boolean;
  tablet?: boolean;
  failure?: boolean;
};

async function activate(control: Locator, touch?: boolean) {
  if (touch) await control.tap();
  else await control.click();
}

async function openRename(page: Page, title: string, scenario: Scenario) {
  if (scenario.entry === "sidebar") {
    await new SessionPage(page).openSidebarMenuAndClick(title, "Rename");
  } else {
    await page.keyboard.press("Control+k");
    const palette = page.getByRole("dialog", { name: "Command Palette", exact: true });
    await palette.getByRole("combobox").fill("Rename");
    const option = palette
      .getByRole("option")
      .filter({ has: page.getByText("Rename", { exact: true }) });
    await expect(option).toHaveCount(1);
    await activate(option, scenario.touch);
  }
  const dialog = page.getByRole("dialog", { name: "Rename task", exact: true });
  await expect(dialog).toBeVisible();
  return dialog;
}

// Real mounted caller, real backend; only the original PATCH transport is held.
export async function checkRenameOwnership(
  fixtures: { testPage: Page; apiClient: ApiClient; seedData: SeedData; backend: BackendContext },
  testInfo: TestInfo,
  scenario: Scenario,
) {
  await withIsolatedRenameFixture(fixtures, async (seedData) => {
    await checkOwnedRenameOwnership({ ...fixtures, seedData }, testInfo, scenario);
  });
}

async function checkOwnedRenameOwnership(
  fixtures: { testPage: Page; apiClient: ApiClient; seedData: SeedData },
  testInfo: TestInfo,
  scenario: Scenario,
) {
  const { testPage: page, apiClient, seedData } = fixtures;
  const options = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
  };
  const taskA = await apiClient.createTask(seedData.workspaceId, "Row source A", options);
  const taskB = await apiClient.createTask(seedData.workspaceId, "Row source B", options);
  let release!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  let admitted!: () => void;
  const admission = new Promise<void>((resolve) => {
    admitted = resolve;
  });
  const requests: unknown[] = [];
  let first = true;
  await page.route(`**/api/v1/tasks/${taskA.id}`, async (route) => {
    if (route.request().method() !== "PATCH") return route.continue();
    requests.push(route.request().postDataJSON());
    if (first) {
      first = false;
      admitted();
      await held;
      if (scenario.failure)
        return route.fulfill({ status: 503, json: { error: "Rename unavailable" } });
    }
    const response = await route.fetch();
    await route.fulfill({ response });
  });
  try {
    await page.goto(`/t/${taskA.id}`);
    await new SessionPage(page).waitForLoad();
    if (scenario.tablet) await page.setViewportSize({ width: 900, height: 1000 });
    const firstDialog = await openRename(page, taskA.title, scenario);
    await firstDialog.getByRole("textbox").fill("  Submitted A  ");
    await activate(firstDialog.getByRole("button", { name: "Save", exact: true }), scenario.touch);
    await admission;
    await expect(firstDialog).toBeHidden();
    expect(requests).toEqual([{ title: "Submitted A" }]);
    expect((await apiClient.getTask(taskA.id)).title).toBe(taskA.title);
    const next = scenario.sameTask ? taskA : taskB;
    const dialog = await openRename(page, next.title, scenario);
    const input = dialog.getByRole("textbox");
    await expect(input).toBeFocused();
    await dialog.evaluate(async () => {
      await Promise.all(
        document
          .getAnimations()
          .filter((animation) => Number.isFinite(animation.effect?.getComputedTiming().iterations))
          .map((animation) => animation.finished.catch(() => undefined)),
      );
    });
    await input.fill("  Exact newer draft  ");
    await input.press("Home");
    await input.press("ArrowRight");
    await input.press("ArrowRight");
    await input.press("ArrowRight");
    await input.press("ArrowRight");
    await input.press("ArrowRight");
    await input.press("ArrowRight");
    await input.press("ArrowRight");
    expect(await input.evaluate((element) => (element as HTMLInputElement).selectionStart)).toBe(7);
    const settled = page.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/v1/tasks/${taskA.id}`) &&
        response.request().method() === "PATCH",
    );
    release();
    const response = await settled;
    expect(response.status()).toBe(scenario.failure ? 503 : 200);
    await response.finished();
    await page.evaluate(
      () =>
        new Promise<void>((resolve) => {
          requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
        }),
    );
    await expect(input).toHaveValue("  Exact newer draft  ");
    expect(await input.evaluate((element) => (element as HTMLInputElement).selectionStart)).toBe(7);
    await testInfo.attach("retained-row-rename", {
      body: await page.screenshot({ path: testInfo.outputPath("retained-row-rename.png") }),
      contentType: "image/png",
    });
    expect((await apiClient.getTask(taskA.id)).title).toBe(
      scenario.failure ? taskA.title : "Submitted A",
    );
    expect((await apiClient.getTask(taskB.id)).title).toBe(taskB.title);
    await expect(page).toHaveURL(new RegExp(`/t/${taskA.id}$`));
    await activate(
      dialog.getByRole("button", { name: scenario.sameTask ? "Cancel" : "Save", exact: true }),
      scenario.touch,
    );
    await expect(dialog).toBeHidden();
    if (!scenario.sameTask) {
      await expect
        .poll(async () => (await apiClient.getTask(taskB.id)).title)
        .toBe("Exact newer draft");
      await page.reload();
      await expect(new SessionPage(page).sidebarTaskItem("Exact newer draft")).toBeVisible();
    } else expect(requests).toHaveLength(1);
  } finally {
    release();
    await page.unrouteAll({ behavior: "wait" });
  }
}
