import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { watchWs } from "../../helpers/causal-waits";

const FILE = "palette-create-check.txt";
const SENTINEL = "original file contents\n";

export async function preparePaletteFile(page: Page, api: ApiClient, seed: SeedData) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Palette file create",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      executor_profile_id: seed.worktreeExecutorProfileId,
      repository_ids: [seed.repositoryId],
    },
  );
  const ws = watchWs(page);
  await page.goto(`/t/${task.id}`);
  await expect
    .poll(async () => (await api.getTaskEnvironment(task.id))?.workspace_path)
    .toBeTruthy();
  const environment = await api.getTaskEnvironment(task.id);
  const root = environment?.workspace_path;
  if (!root) throw new Error("Task workspace path is unavailable");
  return { ws, file: path.join(root, FILE) };
}

export async function openCreateFile(page: Page, mobile: boolean) {
  if (mobile) {
    await page.getByTestId("app-nav-trigger").tap();
    await page.getByTestId("app-nav-sheet").getByRole("button", { name: "Search tasks" }).tap();
    const scope = page.getByRole("tab", { name: "Commands" });
    await expect(scope).toBeVisible();
    await scope.tap();
  } else {
    await page.keyboard.press(process.platform === "darwin" ? "Meta+k" : "Control+k");
  }
  const palette = page.getByRole("dialog", { name: "Command Palette", exact: true });
  const input = palette.getByRole("combobox");
  await input.fill("Create File");
  const option = palette.getByRole("option", { name: "Create File", exact: true });
  await expect(option).toBeVisible();
  if (mobile) {
    expect((await option.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await option.tap();
  } else {
    await option.click();
  }
  await input.fill(FILE);
  return { palette, input };
}

export async function createControl(
  page: Page,
  fixture: Awaited<ReturnType<typeof preparePaletteFile>>,
  mobile: boolean,
) {
  const { palette, input } = await openCreateFile(page, mobile);
  const response = fixture.ws.waitForResponse("workspace.file.create", {
    requestWhere: (payload) => payload.path === FILE,
  });
  await input.press("Enter");
  expect((await response).payload).toMatchObject({ path: FILE, success: true });
  await expect(palette).toBeHidden();
  await expect.poll(() => fs.existsSync(fixture.file)).toBe(true);
  fs.writeFileSync(fixture.file, SENTINEL);
}

export async function duplicateFailure(
  page: Page,
  fixture: Awaited<ReturnType<typeof preparePaletteFile>>,
  mobile: boolean,
) {
  const { palette, input } = await openCreateFile(page, mobile);
  const response = fixture.ws
    .waitForResponse("workspace.file.create", {
      requestWhere: (payload) => payload.path === FILE,
    })
    .then(
      () => "unexpected success",
      (error: Error) => error.message,
    );
  await input.press("Enter");
  expect(await response).toContain("file already exists");
  await expect(palette).toBeHidden();
  expect(fs.readFileSync(fixture.file, "utf8")).toBe(SENTINEL);
  const notice = page.getByTestId("toast-message").filter({ hasText: "Failed to create file" });
  await expect(notice).toBeVisible();
  await expect(notice).toContainText("file already exists");
  return notice;
}
