import { execFileSync } from "node:child_process";
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

async function openRename(page: Page, taskId: string) {
  const picker = page.getByRole("dialog", { name: "Tasks", exact: true });
  if (!(await picker.isVisible())) await page.getByTestId("mobile-task-picker-trigger").tap();
  const row = picker.locator(`[data-task-row-id="${taskId}"]`);
  await expect(row).toHaveCount(1);
  await row.getByRole("button", { name: "Task actions", exact: true }).tap();
  await page.getByRole("menuitem", { name: "Rename", exact: true }).tap();
  const dialog = page.getByRole("dialog", { name: "Rename task", exact: true });
  await expect(dialog).toBeVisible();
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

// @covers AC-UI-EDIT-FOCUS-001.7
for (const { reopenSameTask, tablet } of [
  { reopenSameTask: false, tablet: false },
  { reopenSameTask: true, tablet: false },
  { reopenSameTask: true, tablet: true },
]) {
  test(`phone preserves ${reopenSameTask ? "same-task" : "different-task"} Rename through an earlier response${tablet ? " after tablet resize" : ""}`, async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const seedBefore = repositorySnapshot(seedData.repositoryPath);
    const options = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    };
    const taskA = await apiClient.createTask(seedData.workspaceId, "Rename source A", options);
    const taskB = await apiClient.createTask(seedData.workspaceId, "Rename source B", options);
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
    await testPage.route(`**/api/v1/tasks/${taskA.id}`, async (route) => {
      if (route.request().method() !== "PATCH") return route.continue();
      requests.push(route.request().postDataJSON());
      if (first) {
        first = false;
        admitted();
        await held;
      }
      const response = await route.fetch();
      await route.fulfill({ response });
    });
    try {
      await testPage.goto(`/t/${taskA.id}`);
      await new SessionPage(testPage).waitForLoad();
      const firstDialog = await openRename(testPage, taskA.id);
      await firstDialog.getByRole("textbox").fill("  Submitted A  ");
      await firstDialog.getByRole("button", { name: "Save", exact: true }).tap();
      await admission;
      await expect(firstDialog).toBeHidden();
      expect(requests).toEqual([{ title: "Submitted A" }]);
      expect((await apiClient.getTask(taskA.id)).title).toBe("Rename source A");
      const nextTask = reopenSameTask ? taskA : taskB;
      const dialog = await openRename(testPage, nextTask.id);
      const input = dialog.getByRole("textbox");
      await input.fill("  Exact newer draft  ");
      await expect(input).toHaveValue("  Exact newer draft  ");
      if (tablet) {
        await testPage.setViewportSize({ width: 900, height: 1000 });
        await expect(input).toHaveValue("  Exact newer draft  ");
      }
      const settled = testPage.waitForResponse(
        (response) =>
          response.url().endsWith(`/api/v1/tasks/${taskA.id}`) &&
          response.request().method() === "PATCH",
      );
      release();
      const response = await settled;
      expect(response.status()).toBe(200);
      await response.finished();
      // Cross the browser paint boundary after the fetch body and React completion.
      await testPage.evaluate(
        () =>
          new Promise<void>((resolve) => {
            requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
          }),
      );
      await expect(input).toHaveValue("  Exact newer draft  ");
      await testInfo.attach("retained-rename-draft", {
        body: await testPage.screenshot(),
        contentType: "image/png",
      });
      expect((await apiClient.getTask(taskA.id)).title).toBe("Submitted A");
      expect((await apiClient.getTask(taskB.id)).title).toBe("Rename source B");
      if (reopenSameTask) {
        await dialog.getByRole("button", { name: "Cancel", exact: true }).tap();
        await expect(dialog).toBeHidden();
        expect(requests).toHaveLength(1);
      } else {
        await dialog.getByRole("button", { name: "Save", exact: true }).tap();
        await expect(dialog).toBeHidden();
        await expect
          .poll(async () => (await apiClient.getTask(taskB.id)).title)
          .toBe("Exact newer draft");
      }
    } finally {
      release();
      await testPage.unrouteAll({ behavior: "wait" });
      expect(repositorySnapshot(seedData.repositoryPath)).toEqual(seedBefore);
    }
  });
}
