import fs from "node:fs";
import path from "node:path";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import type { Locator, Page } from "@playwright/test";
import { expect, test, type SeedData } from "../fixtures/test-base";
import type { PrAssetCapture } from "./pr-asset-capture";
import type { ApiClient } from "./api-client";
import { waitForHttp } from "./causal-waits";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

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
  const markerContents = fs.readFileSync(marker);
  fs.writeFileSync(marker, "");
  return {
    task,
    restore: () => {
      fs.rmSync(file, { force: true });
      try {
        fs.writeFileSync(marker, markerContents, { flag: "r+" });
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
      }
    },
    assertRetained: () => {
      expect(digest()).toBe(before);
      expect(fs.readFileSync(marker, "utf8")).toBe("");
      expect(git("worktree", "list", "--porcelain")).toContain(`worktree ${checkout}`);
      expect(git("rev-parse", branch).trim()).toBe(head);
    },
  };
}

export async function deleteDamagedTaskKeepingFiles(page: Page, dialog: Locator, taskID: string) {
  await expect(dialog.getByTestId("delete-unverified-workspace-checkbox")).toBeVisible();
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

export async function deleteDamagedTaskWithPendingCleanup(
  page: Page,
  dialog: Locator,
  taskID: string,
  capture?: PrAssetCapture,
) {
  const acknowledgement = dialog.getByTestId("delete-unverified-workspace-checkbox");
  const action = dialog.getByRole("button", { name: "Delete", exact: true });
  await expect(acknowledgement).toBeVisible();
  await expect(acknowledgement).not.toBeChecked();
  await expect(dialog.getByTestId("delete-keep-files-checkbox")).not.toBeChecked();
  await expect(dialog.getByTestId("delete-discard-worktree-checkbox")).toHaveCount(0);
  await expect(action).toBeDisabled();
  const preview = waitForHttp(page, "POST", /\/tasks\/delete-preflight$/, {
    predicate: (response) =>
      response.request().postDataJSON()?.allow_unverified_workspace_changes === true,
  });
  const mobile = (page.viewportSize()?.width ?? 1024) < 768;
  if (mobile) {
    await acknowledgement.tap();
  } else {
    await acknowledgement.focus();
    await acknowledgement.press("Space");
  }
  expect((await preview).ok()).toBe(true);
  await expect(action).toBeEnabled();
  if (mobile) {
    expect((await acknowledgement.locator("..").boundingBox())?.height).toBeGreaterThanOrEqual(44);
    expect((await action.boundingBox())?.height).toBeGreaterThanOrEqual(44);
  }
  await page.screenshot({ path: test.info().outputPath("pending-cleanup-delete.png") });
  await capture?.screenshot("pending-cleanup-delete", {
    caption: "Delete a task when workspace changes cannot be checked",
  });
  const deletion = waitForHttp(page, "DELETE", new RegExp(`/tasks/${taskID}$`));
  if (mobile) {
    await action.tap();
  } else {
    await action.focus();
    await action.press("Enter");
  }
  const response = await deletion;
  expect(response.ok()).toBe(true);
  expect(await response.json()).toMatchObject({
    success: true,
    workspace_cleanup: "delete",
    workspace_cleanup_pending: true,
  });
  await expect(dialog).not.toBeVisible();
}

export async function assertTasksPagePendingCleanup(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  capture: PrAssetCapture,
) {
  const title = "Tasks page damaged workspace";
  const { task, assertRetained, restore } = await createDamagedDeleteTask(api, seed, title);
  try {
    await page.goto("/tasks?group=none");
    const row = page.getByTestId("tasks-list-row").filter({ hasText: title });
    const action = row.getByRole("button", { name: "Delete task", exact: true });
    if ((page.viewportSize()?.width ?? 1024) < 768) {
      await action.tap();
    } else {
      await action.click();
    }
    await deleteDamagedTaskWithPendingCleanup(page, page.getByRole("alertdialog"), task.id);
    await expect(page.getByText("Task deleted. Cleanup pending.", { exact: true })).toBeVisible();
    await expect(
      page.getByText(
        "Files that cannot be safely removed remain protected. Kandev will retry cleanup.",
        { exact: true },
      ),
    ).toBeVisible();
    await expect(
      page.getByTestId("toast-message").filter({ hasText: "Task deleted. Cleanup pending." }),
    ).toBeInViewport({ ratio: 1 });
    await capture.screenshot("tasks-page-cleanup-pending", {
      caption: "Tasks page confirms deletion and reports protected files pending cleanup.",
    });
    await expect(row).toHaveCount(0);
    expect((await api.rawRequest("GET", `/api/v1/tasks/${task.id}`)).status).toBe(404);
    assertRetained();
    await assertNoDocumentHorizontalOverflow(page, "Tasks page cleanup notice");
  } finally {
    restore();
  }
}
