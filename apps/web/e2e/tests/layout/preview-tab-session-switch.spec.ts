import path from "node:path";
import { expect, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
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

async function seedFinishedTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ id: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
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
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 30_000, message: `Waiting for ${title} session to finish` },
    )
    .toBe(true);
  return task;
}

/** `.dv-tab` is the wrapper dockview toggles `dv-active-tab` on. */
function tabWrapperByText(page: Page, text: string) {
  return page.locator(".dv-tab", { has: page.locator(".dv-default-tab", { hasText: text }) });
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

  test("promoted file tab does not duplicate after task switch round-trip", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // Worker-scoped backend reuses the same repo across tests in the file, so each
    // test seeds its own filename to avoid "nothing to commit" collisions.
    const FILE_PROMOTED = "promoted.ts";
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(FILE_PROMOTED, "// promoted content");
    git.stageAll();
    git.commit("seed promoted");

    const taskA = await seedFinishedTask(apiClient, seedData, "Promote Round-Trip A");
    const taskB = await seedFinishedTask(apiClient, seedData, "Promote Round-Trip B");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.taskCardByTitle("Promote Round-Trip A").click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Open file in preview, then promote via double-click. The panel keeps id
    // `preview:file-editor` with `params.promoted=true` until another file is
    // opened, so we sync on the italic class disappearing rather than the testid.
    await openFileInPreview(testPage, session, FILE_PROMOTED);
    const previewTab = testPage.getByTestId("preview-tab-file-editor");
    await expect(previewTab).toBeVisible({ timeout: 15_000 });
    await previewTab.dblclick();
    await expect(previewTab).not.toHaveClass(/italic/, { timeout: 10_000 });

    // Round-trip: switch to Task B, then back to Task A (forces fromJSON restore).
    await session.clickTaskInSidebar("Promote Round-Trip B");
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskB.id), { timeout: 15_000 });
    await session.waitForLoad();

    await session.clickTaskInSidebar("Promote Round-Trip A");
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskA.id), { timeout: 15_000 });
    await expect(testPage.locator(".dv-dockview")).toBeVisible({ timeout: 15_000 });

    // Regression: previously `loadAndRestoreTabs` re-added `file:promoted.ts` on
    // top of the snapshot's restored `preview:file-editor` (which was persisted
    // as `pinned: true` because it was a promoted preview), producing two tabs
    // with the same filename. Strict count of 1 catches the duplicate.
    const fileTab = testPage.locator(".dv-default-tab").filter({ hasText: FILE_PROMOTED });
    await expect(fileTab).toHaveCount(1, { timeout: 15_000 });
  });

  test("active center tab is preserved across task switch round-trip", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const FILE_ACTIVE = "active-file.ts";
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(FILE_ACTIVE, "// active content");
    git.stageAll();
    git.commit("seed active");

    const taskA = await seedFinishedTask(apiClient, seedData, "Active Tab Round-Trip A");
    const taskB = await seedFinishedTask(apiClient, seedData, "Active Tab Round-Trip B");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.taskCardByTitle("Active Tab Round-Trip A").click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Open file in preview — it auto-activates, but we click it explicitly to
    // make the intent clear and to be robust against any default-active changes.
    await openFileInPreview(testPage, session, FILE_ACTIVE);
    const fileTabWrapper = tabWrapperByText(testPage, FILE_ACTIVE);
    await fileTabWrapper.click();
    await expect(fileTabWrapper).toHaveClass(/dv-active-tab/, { timeout: 10_000 });

    // Round-trip
    await session.clickTaskInSidebar("Active Tab Round-Trip B");
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskB.id), { timeout: 15_000 });
    await session.waitForLoad();

    await session.clickTaskInSidebar("Active Tab Round-Trip A");
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskA.id), { timeout: 15_000 });
    await expect(testPage.locator(".dv-dockview")).toBeVisible({ timeout: 15_000 });

    // Regression: previously `useAutoSessionTab` always called `setActive()` on
    // the session panel after fromJSON, overriding the restored active state.
    // The file tab the user left active must remain active across the round-trip.
    // (Only one tab per group can be `dv-active-tab` so this implicitly asserts
    // the session panel is not active.)
    await expect(fileTabWrapper).toHaveClass(/dv-active-tab/, { timeout: 15_000 });
  });
});
