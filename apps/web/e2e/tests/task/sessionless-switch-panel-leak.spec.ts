import path from "node:path";
import { type Page, expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

type SeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  repositoryId: string;
};

/**
 * Regression: switching from a task with env-scoped panels open (file-editor,
 * diff-viewer, commit-detail, browser, vscode, pr-detail) to a task that
 * needs an env prepared used to leave those panels mounted in the dockview
 * for the entire `await launchSession(...)` round-trip + WS env-id
 * propagation. They surfaced as stray tabs (e.g. a file-editor tab) on the
 * new task's page while it was still preparing.
 *
 * Fix: `prepareAndSwitchTask` calls `releaseLayoutToDefault` BEFORE awaiting
 * the launch, dropping env-scoped portals so the user sees a clean slate.
 */

async function setupTaskWithFilePanel(args: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backendTmpDir: string;
}): Promise<{ session: SessionPage; filename: string }> {
  const git = new GitHelper(
    path.join(args.backendTmpDir, "repos", "e2e-repo"),
    makeGitEnv(args.backendTmpDir),
  );
  const filename = "leak-fixture.ts";
  git.createFile(filename, "// leak fixture\n");
  git.stageAll();
  git.commit("seed leak fixture");

  const profile = await createStandardProfile(args.apiClient, "panel-leak-source");
  await args.apiClient.createTaskWithAgent(
    args.seedData.workspaceId,
    "Panel Leak Source",
    profile.id,
    {
      description: "Source task with env-scoped file panel open",
      workflow_id: args.seedData.workflowId,
      workflow_step_id: args.seedData.startStepId,
      repository_ids: [args.seedData.repositoryId],
    },
  );

  const session = await openTaskSession(args.testPage, "Panel Leak Source");
  await session.clickTab("Files");
  await expect(session.files).toBeVisible({ timeout: 10_000 });

  const node = session.fileTreeNode(filename);
  await expect(node).toBeVisible({ timeout: 15_000 });
  await node.click();
  // The file-editor panel is env-scoped — exactly the kind that used to leak.
  await expect(args.testPage.getByTestId("preview-tab-file-editor")).toBeVisible({
    timeout: 10_000,
  });
  return { session, filename };
}

/** Read the live dockview component names — file-editor / diff-viewer /
 *  commit-detail are env-scoped and must NOT survive into a different task. */
async function readLiveDockviewComponents(testPage: Page): Promise<string[]> {
  return testPage.evaluate(() => {
    type Panel = { id: string; api?: { component?: string } };
    type Api = { panels: Panel[] };
    const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
    if (!api) return [];
    return api.panels.map((p) => p.api?.component ?? "");
  });
}

const ENV_SCOPED_COMPONENTS = [
  "file-editor",
  "diff-viewer",
  "commit-detail",
  "browser",
  "vscode",
  "pr-detail",
];

test.describe("Sessionless task switch — env-scoped panel cleanup", () => {
  test.describe.configure({ retries: 1 });

  test("dropping into a sessionless task releases the previous task's env-scoped panels", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // Source task: open a file-editor panel (env-scoped).
    const { session } = await setupTaskWithFilePanel({
      testPage,
      apiClient,
      seedData,
      backendTmpDir: backend.tmpDir,
    });

    // Sanity: the env-scoped panel is in the live dockview.
    const beforeComponents = await readLiveDockviewComponents(testPage);
    expect(beforeComponents).toContain("file-editor");

    // Sessionless target task — clicking it triggers prepareAndSwitchTask,
    // which awaits launchSession before any layout switch could otherwise run.
    const target = await apiClient.createTask(seedData.workspaceId, "Panel Leak Target", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const targetRow = session.taskInSidebar("Panel Leak Target");
    await expect(targetRow).toBeVisible({ timeout: 10_000 });
    await session.clickTaskInSidebar("Panel Leak Target");

    // Bug invariant: env-scoped panels from the source task must NOT be
    // visible on the target task's page during prepare. Poll a short window
    // — without the fix the file-editor panel survives the entire
    // launchSession round-trip + WS env-id propagation (multi-second).
    await expect
      .poll(
        async () => {
          const components = await readLiveDockviewComponents(testPage);
          return components.some((c) => ENV_SCOPED_COMPONENTS.includes(c));
        },
        {
          timeout: 5_000,
          message: "env-scoped panels from previous task must be released on sessionless switch",
        },
      )
      .toBe(false);

    // And the URL eventually reflects the target task (sanity that the click
    // actually triggered the select flow).
    await expect(testPage).toHaveURL(new RegExp(`/t/${target.id}(?:\\?|$)`), { timeout: 30_000 });
  });
});
