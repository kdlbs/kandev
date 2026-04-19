import path from "node:path";
import { expect, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { KanbanPage } from "../../pages/kanban-page";

const FILE_A = "alpha.ts";
const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

async function openFileInPreview(page: Page, session: SessionPage, filename: string) {
  await session.clickTab("Files");
  await expect(session.files).toBeVisible({ timeout: 10_000 });
  const fileRow = session.files.getByText(filename);
  await expect(fileRow).toBeVisible({ timeout: 15_000 });
  await fileRow.click();
  // Retry click if preview tab didn't appear (executor may need a moment)
  const previewTab = page.getByTestId("preview-tab-file-editor");
  try {
    await expect(previewTab).toBeVisible({ timeout: 5_000 });
  } catch {
    await fileRow.click();
  }
}

test.describe("Preview tab survives session switch", () => {
  test("preview tab persists after switching tasks and back", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // Seed a file in the repo
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(FILE_A, "// alpha content");
    git.stageAll();
    git.commit("seed alpha");

    // Create Task A and wait for it to complete
    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Preview Switch Task A",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(taskA.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for Task A session to finish" },
      )
      .toBe(true);

    // Create Task B and wait for it to complete
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Preview Switch Task B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(taskB.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for Task B session to finish" },
      )
      .toBe(true);

    // Navigate to Task A via kanban
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.taskCardByTitle("Preview Switch Task A").click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Open a file in preview mode
    await openFileInPreview(testPage, session, FILE_A);
    const previewTab = testPage.getByTestId("preview-tab-file-editor");
    await expect(previewTab).toBeVisible({ timeout: 15_000 });

    // Switch to Task B via sidebar
    await session.clickTaskInSidebar("Preview Switch Task B");
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskB.id), {
      timeout: 15_000,
    });
    await session.waitForLoad();

    // Preview tab should not be visible on Task B
    await expect(previewTab).not.toBeVisible({ timeout: 5_000 });

    // Switch back to Task A — slow path restores layout via fromJSON
    await session.clickTaskInSidebar("Preview Switch Task A");
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskA.id), {
      timeout: 15_000,
    });
    // Wait for layout to stabilize after fromJSON restore
    await expect(testPage.locator(".dv-dockview")).toBeVisible({ timeout: 15_000 });
    await testPage.waitForTimeout(1_000);

    // File tab should be restored (preview or pinned — the file was open)
    const fileTab = testPage.locator(".dv-default-tab").filter({ hasText: FILE_A });
    await expect(fileTab).toHaveCount(1, { timeout: 15_000 });
  });
});
