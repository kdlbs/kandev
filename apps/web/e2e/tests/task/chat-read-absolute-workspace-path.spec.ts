import path from "node:path";
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { waitForWorkspacePath } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

function observeFileTreeRequestPaths(page: Page): string[] {
  const paths: string[] = [];
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", ({ payload }) => {
      const text = typeof payload === "string" ? payload : payload.toString();
      try {
        const frame = JSON.parse(text) as {
          action?: string;
          payload?: { path?: unknown };
        };
        if (frame.action === "workspace.tree.get" && typeof frame.payload?.path === "string") {
          paths.push(frame.payload.path);
        }
      } catch {
        // Gateway-adjacent sockets can carry non-JSON frames.
      }
    });
  });
  return paths;
}

test.describe("Absolute workspace read links", () => {
  // @covers AC-UI-FILE-TREE-PATH-SCOPE-001.1
  // @covers AC-UI-FILE-TREE-PATH-SCOPE-001.3
  test("opens with a relative editor and file-tree identity", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);
    const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const directory = `absolute-read-${suffix}`;
    const fileName = "case.ts";
    const filePath = `${directory}/${fileName}`;
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(filePath, 'export const suspect = "Colonel Mustard";');
    git.stageAll();
    git.commit(`add ${filePath}`);

    const profile = await createStandardProfile(apiClient, `absolute-read-${Date.now()}`);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Absolute Workspace Read",
      profile.id,
      {
        description: 'e2e:message("ready")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    const workspacePath = await waitForWorkspacePath(apiClient, task.id, task.session_id);
    const absoluteFilePath = path.join(workspacePath, filePath);
    await apiClient.seedSessionMessage(task.session_id, {
      type: "tool_read",
      content: "Read file",
      metadata: {
        status: "complete",
        tool_call_id: "tc-absolute-workspace-read",
        normalized: {
          read_file: {
            file_path: absoluteFilePath,
            offset: 1,
            limit: 1,
            output: { content: 'export const suspect = "Colonel Mustard";', line_count: 1 },
          },
        },
      },
    });

    const treeRequestPaths = observeFileTreeRequestPaths(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });

    const link = session.activeChat().locator(`button[title="${absoluteFilePath}"]`);
    await expect(link).toBeVisible({ timeout: 15_000 });
    await link.click();
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.locator(".dv-tab.dv-active-tab", { hasText: fileName })).toBeVisible();

    await session.clickTab("Files");
    await expect(session.fileTreeNode(filePath)).toBeVisible({ timeout: 15_000 });
    expect(treeRequestPaths.length).toBeGreaterThan(0);
    expect(treeRequestPaths.every((requestPath) => !path.isAbsolute(requestPath))).toBe(true);
  });
});
