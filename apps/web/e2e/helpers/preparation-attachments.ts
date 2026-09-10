import fs from "node:fs";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { makeGitEnv } from "./git-helper";
import { KanbanPage } from "../pages/kanban-page";
import { MobileKanbanPage } from "../pages/mobile-kanban-page";

type Options = {
  testPage: Page;
  apiClient: ApiClient;
  backend: BackendContext;
  seedData: SeedData;
  testInfo: TestInfo;
  mobile?: boolean;
  failPreparation?: boolean;
};

export async function assertPreparationAttachments(options: Options) {
  const {
    testPage: page,
    apiClient,
    backend,
    seedData,
    testInfo,
    mobile,
    failPreparation,
  } = options;
  const fixture = fs.mkdtempSync(path.join(backend.tmpDir, "attachment-preview-"));
  const repoDir = path.join(fixture, "repo");
  const entered = path.join(fixture, "entered");
  const release = path.join(fixture, "release");
  const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
  const { settings } = await apiClient.getUserSettings();
  let taskId: string | undefined;
  let repositoryId: string | undefined;
  try {
    execFileSync("git", ["clone", "--local", seedData.repositoryPath, repoDir], {
      env: makeGitEnv(backend.tmpDir),
      stdio: "ignore",
    });
    const repository = await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "Preparation preview repository",
      pull_before_worktree: false,
    });
    repositoryId = repository.id;
    await apiClient.updateRepository(repository.id, {
      setup_script: `touch ${quote(entered)}\nwhile [ ! -f ${quote(release)} ]; do sleep 0.1; done\n${failPreparation ? "exit 1" : "true"}`,
    });
    await apiClient.saveUserSettings({
      task_create_last_used: {
        repository_id: repository.id,
        branch: "main",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        workflow_ids_by_workspace: { [seedData.workspaceId]: seedData.workflowId },
      },
    });
    if (mobile) {
      const kanban = new MobileKanbanPage(page);
      await kanban.goto();
      await kanban.mobileFab.tap();
    } else {
      const kanban = new KanbanPage(page);
      await kanban.goto();
      await kanban.createTaskButton.first().click();
    }
    const dialog = page.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("task-title-input").fill("Preparation attachment previews");
    await dialog.getByTestId("task-description-input").fill("/e2e:simple-message");
    await expect(dialog.getByTestId("repo-chip-trigger").first()).toContainText(
      "Preparation preview repository",
    );
    const imageBytes = fs.readFileSync(
      path.join(process.cwd(), "public/web-app-manifest-192x192.png"),
    );
    await dialog.locator('input[type="file"]').setInputFiles([
      { name: "screen-one.png", mimeType: "image/png", buffer: imageBytes },
      { name: "screen-two.png", mimeType: "image/png", buffer: imageBytes },
      { name: "notes.txt", mimeType: "text/plain", buffer: Buffer.from("preview file contents") },
    ]);
    const submit = dialog.getByTestId("submit-start-agent");
    await expect(submit).toBeEnabled();
    const responsePromise = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
    );
    await submit.click();
    const response = await responsePromise;
    expect(response.ok()).toBeTruthy();
    const created = (await response.json()) as { id: string; session_id: string };
    taskId = created.id;
    expect(created.session_id).toBeTruthy();
    await expect.poll(() => fs.existsSync(entered), { timeout: 60_000 }).toBe(true);
    await page.goto(`/t/${taskId}`);
    const bubble = page.getByTestId("user-message-bubble");
    await expect(bubble).toHaveCount(1);
    await expect(bubble).toContainText("/e2e:simple-message");
    await expect(
      bubble.getByRole("button", { name: "Open Attachment 1", exact: true }),
    ).toBeVisible();
    await expect(
      bubble.getByRole("button", { name: "Open Attachment 2", exact: true }),
    ).toBeVisible();
    for (const index of [1, 2]) {
      const trigger = bubble.getByRole("button", { name: `Open Attachment ${index}`, exact: true });
      if (mobile) await trigger.tap();
      else await trigger.click();
      await expect(page.getByRole("dialog", { name: "Image preview" })).toBeVisible();
      await page.keyboard.press("Escape");
    }
    const file = bubble.getByTestId("message-file-attachment");
    await expect(file).toContainText("notes.txt");
    await page.reload();
    await expect(
      bubble.getByRole("button", { name: "Open Attachment 2", exact: true }),
    ).toBeVisible();
    await expect(file).toContainText("notes.txt");
    const before = await apiClient.listSessionMessages(created.session_id);
    expect(before.messages.filter((message) => message.author_type === "user")).toHaveLength(0);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await page.screenshot({ path: testInfo.outputPath("preparation-attachments.png") });

    // An unavailable image must leave the text and remaining attachments usable.
    const firstSource = await bubble
      .getByRole("button", { name: "Open Attachment 1", exact: true })
      .locator("img")
      .getAttribute("src");
    expect(firstSource).toBeTruthy();
    await page.route(firstSource!, (route) => route.fulfill({ status: 404, body: "not found" }));
    await page.reload();
    await expect(bubble).toContainText("/e2e:simple-message");
    await expect(file).toBeVisible();
    await page.unroute(firstSource!);

    fs.writeFileSync(release, "release");
    if (failPreparation) {
      const panel = page.getByTestId("prepare-progress-panel");
      await expect(panel).toHaveAttribute("data-status", "completed_with_warnings", {
        timeout: 60_000,
      });
      await page.reload();
      await expect(
        bubble.getByRole("button", { name: "Open Attachment 2", exact: true }),
      ).toBeVisible();
    } else {
      await expect
        .poll(
          async () => {
            const { messages } = await apiClient.listSessionMessages(created.session_id);
            return messages.filter((message) => message.author_type === "user").length;
          },
          { timeout: 60_000 },
        )
        .toBe(1);
      await expect(bubble).toHaveCount(1);
      await page.reload();
      await expect(bubble).toHaveCount(1);
      await expect(
        bubble.getByRole("button", { name: "Open Attachment 2", exact: true }),
      ).toBeVisible();
    }
  } finally {
    fs.writeFileSync(release, "release");
    if (taskId) {
      const response = await apiClient.rawRequest(
        "DELETE",
        `/api/v1/tasks/${taskId}?discard_worktree_changes=true`,
      );
      expect(response.ok).toBeTruthy();
    }
    if (repositoryId) {
      const response = await apiClient.rawRequest("DELETE", `/api/v1/repositories/${repositoryId}`);
      expect(response.ok).toBeTruthy();
    }
    await apiClient.saveUserSettings({
      task_create_last_used: settings.task_create_last_used as Parameters<
        ApiClient["saveUserSettings"]
      >[0]["task_create_last_used"],
    });
    fs.rmSync(fixture, { recursive: true, force: true });
  }
}
