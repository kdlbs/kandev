import { type Page } from "@playwright/test";
import fs from "node:fs";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import type { FileTreePage } from "../../pages/file-tree-page";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { watchWs } from "../../helpers/causal-waits";

// DnD in file-browser.tsx uses native HTML5 drag events (dragstart, dragover,
// drop) keyed off React's onDragStart/Over/Drop. Playwright's locator.dragTo()
// does not trigger native HTML5 DnD reliably in Chromium - the drop target
// must see dragover with preventDefault() and a drop event with the same
// DataTransfer that was set in dragstart.
//
// We dispatch the events manually through locators, constructing a shared
// DataTransfer for the dragstart -> drop sequence. This is the established
// workaround for testing HTML5 DnD in Playwright and mirrors what the user
// would do.

async function setupTask({
  testPage,
  apiClient,
  seedData,
  profileName,
  taskTitle,
  fixtureFiles,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: {
    workspaceId: string;
    workflowId: string;
    startStepId: string;
    repositoryId: string;
    repositoryPath: string;
  };
  profileName: string;
  taskTitle: string;
  fixtureFiles: Record<string, string>;
}) {
  const seededBranch = execFileSync("git", ["symbolic-ref", "--short", "HEAD"], {
    cwd: seedData.repositoryPath,
    encoding: "utf8",
  }).trim();
  const profile = await createStandardProfile(apiClient, profileName);
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, taskTitle, profile.id, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repositories: [{ repository_id: seedData.repositoryId, base_branch: seededBranch }],
  });

  // The task API returns before local workspace preparation finishes. Wait
  // for the environment's durable ready state before the first tree request;
  // otherwise the tree can legitimately snapshot the repository while the
  // agent session is still being attached to it.
  let workspacePath = "";
  await expect
    .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status ?? null, {
      timeout: 30_000,
      message: `Waiting for ${taskTitle} task environment to be ready`,
    })
    .toBe("ready");

  // Seed the task checkout only after its path is published and materialized.
  await expect
    .poll(
      async () => {
        const environment = await apiClient.getTaskEnvironment(task.id);
        const repositoryPaths = (environment?.repos ?? [])
          .filter(
            (repository) =>
              !repository.repository_id || repository.repository_id === seedData.repositoryId,
          )
          .map((repository) => repository.worktree_path);
        const candidates = [
          ...repositoryPaths,
          environment?.workspace_path,
          environment?.worktree_path,
        ].filter(
          (candidate, index, paths): candidate is string =>
            Boolean(candidate) && paths.indexOf(candidate) === index,
        );
        workspacePath =
          candidates.find((candidate) => {
            try {
              return fs.statSync(candidate).isDirectory();
            } catch {
              return false;
            }
          }) ?? "";
        return workspacePath !== "";
      },
      { timeout: 30_000, message: `Waiting for the ${taskTitle} repository worktree` },
    )
    .toBe(true);

  for (const [relativePath, contents] of Object.entries(fixtureFiles)) {
    const absolutePath = path.join(workspacePath, relativePath);
    fs.mkdirSync(path.dirname(absolutePath), { recursive: true });
    fs.writeFileSync(absolutePath, contents);
  }

  try {
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.clickTab("Files");
    return { session, workspacePath, sessionId: task.session_id };
  } catch (error) {
    removeFixtureFiles(workspacePath, Object.keys(fixtureFiles));
    throw error;
  }
}

function removeFixtureFiles(workspacePath: string, fixturePaths: string[]) {
  for (const relativePath of fixturePaths) {
    const absolutePath = path.join(workspacePath, relativePath);
    if (fs.existsSync(absolutePath) && fs.statSync(absolutePath).isFile()) {
      fs.unlinkSync(absolutePath);
    }
    let directory = path.dirname(absolutePath);
    while (directory !== workspacePath) {
      try {
        fs.rmdirSync(directory);
      } catch {
        break;
      }
      directory = path.dirname(directory);
    }
  }
}

async function dispatchHtmlDnd(
  testPage: Page,
  fileTree: Pick<FileTreePage, "waitForFileTreeNode">,
  sourcePath: string,
  targetPath: string,
) {
  // Virtualized trees can unmount the source while the target is revealed.
  // Keep the browser DataTransfer on the page between the two scrolls so the
  // source and target do not need to be mounted at the same time.
  const source = await fileTree.waitForFileTreeNode(sourcePath, 45_000);
  await expect(source).toBeVisible({ timeout: 45_000 });
  await source.scrollIntoViewIfNeeded();
  await source.evaluate((row) => {
    const dataTransfer = new DataTransfer();
    const event = new DragEvent("dragstart", {
      bubbles: true,
      cancelable: true,
      composed: true,
      dataTransfer,
    });
    row.dispatchEvent(event);
    Object.defineProperty(window, "__kandevE2eDataTransfer", {
      configurable: true,
      value: dataTransfer,
    });
  });

  const target = await fileTree.waitForFileTreeNode(targetPath, 45_000);
  await expect(target).toBeVisible({ timeout: 45_000 });
  await target.scrollIntoViewIfNeeded();
  await target.evaluate((row, sourcePath) => {
    const dataTransfer = (window as Window & { __kandevE2eDataTransfer?: DataTransfer })
      .__kandevE2eDataTransfer;
    if (!dataTransfer) throw new Error("DnD source did not establish a DataTransfer");
    const fireOn = (element: Element, type: string) => {
      element.dispatchEvent(
        new DragEvent(type, { bubbles: true, cancelable: true, composed: true, dataTransfer }),
      );
    };
    fireOn(row, "dragenter");
    fireOn(row, "dragover");
    fireOn(row, "drop");
    const source = Array.from(document.querySelectorAll('[data-testid="file-tree-node"]')).find(
      (element) => element.getAttribute("data-path") === sourcePath,
    );
    if (source) fireOn(source, "dragend");
    document.dispatchEvent(new DragEvent("dragend", { bubbles: true, dataTransfer }));
    delete (window as Window & { __kandevE2eDataTransfer?: DataTransfer }).__kandevE2eDataTransfer;
  }, sourcePath);
}

test.describe("File tree drag and drop", () => {
  test.describe.configure({ timeout: 180_000 });

  test.beforeEach(async ({ backend, testPage }) => {
    void testPage;
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.exec("git checkout -f main");
    git.exec("git clean -fd");
  });

  test("drag a file into a folder moves it on disk and in the tree", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixtureFiles = { "movable.ts": "m", "target-dir/keep.ts": "k" };
    const ws = watchWs(testPage);
    const { session, workspacePath } = await setupTask({
      testPage,
      apiClient,
      seedData,
      profileName: "ft-dnd-move",
      taskTitle: "FT DnD Move",
      fixtureFiles,
    });
    try {
      const renameResponse = ws.waitForResponse("workspace.file.rename");
      await dispatchHtmlDnd(testPage, session.fileTree, "movable.ts", "target-dir");
      await renameResponse;
      await session.clickTab("Files");

      await expect(session.fileTreeNode("movable.ts")).toHaveCount(0, { timeout: 10_000 });
      await session.fileTreeNode("target-dir").click();
      await expect(session.fileTreeNode("target-dir/movable.ts")).toBeVisible({ timeout: 10_000 });

      expect(fs.existsSync(path.join(workspacePath, "target-dir", "movable.ts"))).toBe(true);
      expect(fs.existsSync(path.join(workspacePath, "movable.ts"))).toBe(false);
    } finally {
      removeFixtureFiles(workspacePath, [
        "movable.ts",
        "target-dir/movable.ts",
        "target-dir/keep.ts",
      ]);
    }
  });

  test("selected directory carries selected and unselected descendants", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    // @covers AC-UI-FILE-MOVE-SELECTION-001.1 AC-UI-FILE-MOVE-SELECTION-001.2
    const fixtureFiles = {
      "src/a.txt": "selected child sentinel",
      "src/sub/keep.txt": "unselected nested sentinel",
      "dest/keep.txt": "destination sentinel",
    };

    type Frame = {
      id: string;
      type: string;
      action: string;
      payload: { session_id?: string; old_path?: string; new_path?: string; success?: boolean };
    };
    const requests: Frame[] = [];
    const replies = new Map<string, Frame>();
    const moveScope = { sessionId: "" };
    testPage.on("websocket", (socket) => {
      if (!socket.url().endsWith("/ws")) return;
      socket.on("framesent", ({ payload }) => {
        const frame = JSON.parse(String(payload)) as Frame;
        if (
          frame.action === "workspace.file.rename" &&
          frame.payload.session_id === moveScope.sessionId
        )
          requests.push(frame);
      });
      socket.on("framereceived", ({ payload }) => {
        const frame = JSON.parse(String(payload)) as Frame;
        if (requests.some((request) => request.id === frame.id)) replies.set(frame.id, frame);
      });
    });
    const { session, workspacePath, sessionId } = await setupTask({
      testPage,
      apiClient,
      seedData,
      profileName: "ft-directory-selection",
      taskTitle: "FT Directory Selection",
      fixtureFiles,
    });
    try {
      expect(sessionId).toBeTruthy();
      moveScope.sessionId = sessionId!;
      expect(fs.readFileSync(path.join(workspacePath, "src/a.txt"), "utf8")).toBe(
        "selected child sentinel",
      );
      await session.fileTree.waitForFileTreeNode("src");
      await session.fileTreeNode("src").click();
      await session.fileTree.waitForFileTreeNode("src/a.txt");
      await session.fileTreeNode("src/a.txt").click({ modifiers: ["Control"] });
      await session.fileTreeNode("src").click({ modifiers: ["Control"] });
      await expect(session.fileTreeNode("src/a.txt")).toHaveAttribute("data-selected", "true");
      await expect(session.fileTreeNode("src")).toHaveAttribute("data-selected", "true");
      await dispatchHtmlDnd(testPage, session.fileTree, "src/a.txt", "dest");
      await expect
        .poll(() => requests.length > 0 && requests.every(({ id }) => replies.has(id)), {
          message: "Waiting for actual directory-selection rename replies",
        })
        .toBe(true);
      const commandEvidence = testInfo.outputPath("directory-selection-commands.json");
      fs.writeFileSync(
        commandEvidence,
        JSON.stringify({ workspacePath, requests, replies: [...replies.values()] }, null, 2),
      );
      await testInfo.attach("directory-selection-commands", {
        path: commandEvidence,
        contentType: "application/json",
      });

      expect(fs.existsSync(path.join(workspacePath, "src"))).toBe(false);
      expect(fs.readFileSync(path.join(workspacePath, "dest/src/a.txt"), "utf8")).toBe(
        "selected child sentinel",
      );
      expect(fs.readFileSync(path.join(workspacePath, "dest/src/sub/keep.txt"), "utf8")).toBe(
        "unselected nested sentinel",
      );
      expect(fs.existsSync(path.join(workspacePath, "dest/a.txt"))).toBe(false);
      expect(fs.readFileSync(path.join(workspacePath, "dest/keep.txt"), "utf8")).toBe(
        "destination sentinel",
      );
      expect(requests.map(({ payload }) => payload)).toEqual([
        { session_id: sessionId, old_path: "src", new_path: "dest/src" },
      ]);
      expect(replies.get(requests[0].id)?.payload.success).toBe(true);
      // Re-select Files if a Git update activated Changes during the move.
      await session.clickTab("Files");
      await session.fileTreeNode("dest").click();
      await session.fileTree.waitForFileTreeNode("dest/src");
      await session.fileTreeNode("dest/src").click();
      await expect(session.fileTreeNode("dest/src/a.txt")).toBeVisible();
      await session.fileTreeNode("dest/src/sub").click();
      await expect(session.fileTreeNode("dest/src/sub/keep.txt")).toBeVisible();
      await expect(session.fileTreeNode("dest/a.txt")).toHaveCount(0);
    } finally {
      removeFixtureFiles(workspacePath, [
        ...Object.keys(fixtureFiles),
        "dest/src/a.txt",
        "dest/src/sub/keep.txt",
      ]);
    }
  });

  test("drop is rejected when dragging a folder onto itself", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixtureFiles = { "selfdir/leaf.ts": "leaf" };
    const { session, workspacePath } = await setupTask({
      testPage,
      apiClient,
      seedData,
      profileName: "ft-dnd-self",
      taskTitle: "FT DnD Self Reject",
      fixtureFiles,
    });
    try {
      await session.fileTree.waitForFileTreeNode("selfdir");
      await dispatchHtmlDnd(testPage, session.fileTree, "selfdir", "selfdir");

      await expect(session.fileTreeNode("selfdir")).toBeVisible({ timeout: 5_000 });
      await session.fileTreeNode("selfdir").click();
      await expect(session.fileTreeNode("selfdir/leaf.ts")).toBeVisible({ timeout: 10_000 });

      expect(fs.existsSync(path.join(workspacePath, "selfdir", "selfdir"))).toBe(false);
    } finally {
      removeFixtureFiles(workspacePath, ["selfdir/leaf.ts"]);
    }
  });
});
