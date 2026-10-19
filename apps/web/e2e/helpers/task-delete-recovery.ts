import fs from "node:fs";
import path from "node:path";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import type { Locator, Page } from "@playwright/test";
import { expect, test, type SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { waitForHttp } from "./causal-waits";

export async function createDamagedDeleteTask(api: ApiClient, seed: SeedData, title: string) {
  const task = await api.createTaskWithAgent(seed.workspaceId, title, seed.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
    executor_profile_id: seed.worktreeExecutorProfileId,
  });
  await expect
    .poll(async () => (await api.getTaskEnvironment(task.id))?.status, { timeout: 60_000 })
    .toBe("ready");
  await waitForDeletePreviewTask(api, task.id);
  const env = await api.getTaskEnvironment(task.id);
  const checkout = env?.repos?.[0]?.worktree_path ?? env?.worktree_path;
  if (!checkout) throw new Error("test task has no checkout");
  const file = path.join(checkout, "keep-after-delete.txt");
  fs.writeFileSync(file, "Retain this exact content after deleting the damaged task.\n");
  const digest = () => createHash("sha256").update(fs.readFileSync(file)).digest("hex");
  const before = digest();
  const git = (...args: string[]) =>
    execFileSync("git", ["-C", seed.repositoryPath, ...args], { encoding: "utf8" });
  const branch = execFileSync("git", ["-C", checkout, "symbolic-ref", "HEAD"], {
    encoding: "utf8",
  }).trim();
  const head = git("rev-parse", branch).trim();
  const marker = path.join(path.dirname(checkout), ".kandev-workspace.json");
  expect(fs.existsSync(marker)).toBe(true);
  fs.writeFileSync(marker, "");
  return {
    task,
    assertRetained: () => {
      expect(digest()).toBe(before);
      expect(fs.readFileSync(marker, "utf8")).toBe("");
      expect(git("worktree", "list", "--porcelain")).toContain(`worktree ${checkout}`);
      expect(git("rev-parse", branch).trim()).toBe(head);
    },
  };
}

export async function deleteDamagedTaskKeepingFiles(page: Page, dialog: Locator, taskID: string) {
  await expect(dialog.getByTestId("delete-preflight-error")).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Delete", exact: true })).toBeDisabled();
  const preview = waitForHttp(page, "POST", /\/tasks\/delete-preflight$/, {
    predicate: (response) => response.request().postDataJSON()?.workspace_cleanup === "retain",
  });
  await dialog.getByTestId("delete-keep-files-checkbox").click();
  expect((await preview).ok()).toBe(true);
  const action = dialog.getByRole("button", { name: "Delete task and keep files", exact: true });
  await expect(action).toBeEnabled();
  if ((page.viewportSize()?.width ?? 1024) < 768) {
    expect((await action.boundingBox())?.height).toBeGreaterThanOrEqual(44);
  }
  await page.screenshot({ path: test.info().outputPath("keep-files-delete.png") });
  const deletion = waitForHttp(page, "DELETE", new RegExp(`/tasks/${taskID}$`));
  await action.click();
  const response = await deletion;
  expect(response.ok()).toBe(true);
  expect(await response.json()).toMatchObject({ success: true, workspace_cleanup: "retain" });
}

export async function waitForDeletePreviewTask(api: ApiClient, taskID: string) {
  await expect
    .poll(
      async () => {
        const { sessions } = await api.listTaskSessions(taskID);
        return (
          sessions.length > 0 &&
          sessions.every((session) => ["WAITING_FOR_INPUT", "COMPLETED"].includes(session.state))
        );
      },
      { timeout: 60_000, message: "initial agent turn must finish before deletion preview" },
    )
    .toBe(true);
}
