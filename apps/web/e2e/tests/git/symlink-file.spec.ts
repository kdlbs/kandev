import fs from "node:fs";
import path from "node:path";
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { watchWs } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

/**
 * Seed a task with the simple-message scenario and navigate to its session page.
 */
async function seedSimpleTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  requiredPaths: string[] = [],
): Promise<{ session: SessionPage; sessionId: string }> {
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

  // Environment readiness and repository materialization are separate
  // transitions. Wait for the exact fixture paths before the first Files
  // request, otherwise a valid early tree snapshot can be retained while the
  // checkout is still being populated.
  if (requiredPaths.length > 0) {
    let workspacePath = "";
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status ?? null, {
        timeout: 30_000,
        message: `Waiting for ${title} task environment to be ready`,
      })
      .toBe("ready");
    await expect
      .poll(
        async () => {
          const environment = await apiClient.getTaskEnvironment(task.id);
          workspacePath =
            environment?.workspace_path ?? environment?.repos?.[0]?.worktree_path ?? "";
          return requiredPaths.every((requiredPath) =>
            Boolean(workspacePath && fs.existsSync(path.join(workspacePath, requiredPath))),
          );
        },
        {
          timeout: 60_000,
          message: `Waiting for ${requiredPaths.join(", ")} in the ${title} worktree`,
        },
      )
      .toBe(true);
  }

  const gateway = requiredPaths.length > 0 ? watchWs(testPage) : null;
  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  if (gateway) {
    const treeResponse = gateway.waitForResponse("workspace.tree.get");
    await testPage.reload();
    await session.waitForLoad();
    await treeResponse;
  }

  return { session, sessionId: task.session_id ?? task.id };
}

/**
 * Seed a task using the symlink-file-setup mock scenario. The scenario creates
 * real-file.txt, a symlink link-file.txt → real-file.txt, commits both, then
 * modifies real-file.txt leaving an uncommitted diff.
 */
async function seedSymlinkDiffTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Symlink Diff E2E",
    seedData.agentProfileId,
    {
      description: "/e2e:symlink-file-setup",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await expect(session.chat.getByText("symlink-file-setup complete", { exact: false })).toBeVisible(
    { timeout: 45_000 },
  );

  return session;
}

test.describe("Symlink file handling", () => {
  test.describe.configure({ retries: 1 });

  test("symlink to a directory appears as expandable folder in the file tree", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Create a directory with a child file, and a symlink pointing to it.
    // BEFORE navigating so the file tree picks it up on initial load.
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");

    fs.mkdirSync(path.join(repoDir, "real-dir"), { recursive: true });
    fs.writeFileSync(path.join(repoDir, "real-dir", "child.txt"), "inside symlinked dir\n");
    fs.symlinkSync("real-dir", path.join(repoDir, "link-dir"));

    const { session } = await seedSimpleTask(
      testPage,
      apiClient,
      seedData,
      "Symlink Dir Tree Test",
      ["real-dir/child.txt", "link-dir/child.txt"],
    );

    // Open Files tab
    await session.clickTab("Files");
    await expect(session.files).toBeVisible({ timeout: 5_000 });

    // The symlink-to-directory should appear in the tree
    const linkDirNode = await session.fileTree.waitForFileTreeNode("link-dir");
    await expect(linkDirNode.getByTestId("symlink-indicator")).toBeVisible();

    // Click to expand — if the fix is missing, this is classified as a file
    // and would try to open it in the editor instead of expanding.
    await linkDirNode.click();

    // Assert the child file inside the symlinked directory is now visible
    await session.fileTree.waitForFileTreeNode("link-dir/child.txt");
  });

  for (const provider of ["monaco", "codemirror"] as const) {
    test(`clicking a symlink identifies it in the ${provider} editor`, async ({
      testPage,
      apiClient,
      seedData,
      backend,
    }) => {
      const initial = await apiClient.getUserSettings();
      const initialLayout = initial.settings.changes_panel_layout === "tree" ? "tree" : "flat";
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        changes_panel_layout: provider === "monaco" ? "flat" : "tree",
      });
      try {
        await testPage.addInitScript((provider) => {
          localStorage.setItem(
            "kandev-editor-providers",
            JSON.stringify({
              version: 3,
              state: {
                providers: {
                  "code-editor": provider,
                  "diff-viewer": "pierre-diffs",
                  "chat-code-block": "shiki",
                  "chat-diff": "pierre-diffs",
                  "plan-editor": "tiptap",
                },
              },
            }),
          );
        }, provider);
        const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
        fs.rmSync(path.join(repoDir, "link-file.txt"), { force: true });
        fs.writeFileSync(path.join(repoDir, "real-file.txt"), "Hello from symlink target!\n");
        fs.symlinkSync("real-file.txt", path.join(repoDir, "link-file.txt"));

        const { session } = await seedSimpleTask(
          testPage,
          apiClient,
          seedData,
          "Symlink File Tree Test",
          ["real-file.txt", "link-file.txt"],
        );

        await session.clickTab("Changes");
        const changedLink = testPage.getByTestId("file-row-link-file.txt");
        await expect(changedLink).toBeVisible({ timeout: 15_000 });
        await expect(changedLink.getByTestId("symlink-indicator")).toBeVisible();
        await changedLink.hover();
        await expect(changedLink.getByTestId("symlink-indicator")).toBeVisible();
        await expect(
          testPage.getByTestId("file-row-real-file.txt").getByTestId("symlink-indicator"),
        ).toHaveCount(0);

        // Open Files tab and click the symlink
        await session.clickTab("Files");
        await expect(session.files).toBeVisible({ timeout: 5_000 });

        await expect(
          session.fileTreeNode("link-file.txt").getByTestId("symlink-indicator"),
        ).toBeVisible();
        await expect(
          session.fileTreeNode("real-file.txt").getByTestId("symlink-indicator"),
        ).toHaveCount(0);
        const fileRow = session.files.getByText("link-file.txt");
        await expect(fileRow).toBeVisible({ timeout: 10_000 });
        await fileRow.click();

        // Assert editor tab opens for the symlink
        const editorTab = testPage.locator(".dv-default-tab:has-text('link-file.txt')");
        await expect(editorTab).toBeVisible({ timeout: 10_000 });

        // Assert the file content is visible in the Monaco editor
        const editorContent = testPage.locator(
          provider === "monaco" ? ".view-lines" : ".cm-content",
        );
        await expect(editorContent).toContainText("Hello from symlink target", {
          timeout: 10_000,
        });
        await expect(
          testPage
            .locator('[data-testid="symlink-indicator"]:not([data-testid="files-panel"] *)')
            .filter({ visible: true }),
        ).toContainText("Symlink");
        await session.files.getByText("real-file.txt", { exact: true }).click();
        await expect(testPage.locator(".dv-default-tab:has-text('real-file.txt')")).toBeVisible();
        await expect(
          testPage
            .locator('[data-testid="symlink-indicator"]:not([data-testid="files-panel"] *)')
            .filter({ visible: true }),
        ).toHaveCount(0);
      } finally {
        await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
          changes_panel_layout: initialLayout,
        });
      }
    });
  }

  test("clicking a symlink target diff in the Changes panel opens the diff viewer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedSymlinkDiffTask(testPage, apiClient, seedData);

    // Open Changes tab
    const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
    await expect(changesTab).toBeVisible({ timeout: 10_000 });
    await changesTab.click();

    // Click real-file.txt in the changes list (has uncommitted diff)
    const fileRow = testPage
      .getByTestId("unstaged-file-tree")
      .getByTestId("file-row-real-file.txt");
    await expect(fileRow).toBeVisible({ timeout: 10_000 });
    await fileRow.click();

    // Assert Pierre Diffs viewer appears with the modification
    await expect(testPage.locator("diffs-container")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("Modified symlink target", { exact: false })).toBeVisible({
      timeout: 60_000,
    });
  });
});
