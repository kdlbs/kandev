import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import { expect, type SeedData } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";
import type { WsWatcher } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

const ROOT_FILE_CONTENT = "workspace-root-file-marker";

export async function createWorkspaceRootFilesFixture({
  testPage,
  apiClient,
  seedData,
  backend,
  watcher,
}: {
  testPage: import("@playwright/test").Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  watcher: WsWatcher;
}) {
  const gitEnv = makeGitEnv(backend.tmpDir);
  const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const extraRepoDir = path.join(backend.tmpDir, "repos", `root-files-${suffix}`);
  fs.mkdirSync(extraRepoDir, { recursive: true });
  execSync("git init -b main", { cwd: extraRepoDir, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: extraRepoDir, env: gitEnv });
  const extraRepo = await apiClient.createRepository(seedData.workspaceId, extraRepoDir, "main", {
    name: `root-files-${suffix}`,
    pull_before_worktree: false,
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `Workspace Root Files ${suffix}`,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId, extraRepo.id],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle();

  if (!task.session_id) throw new Error("Expected a session for the workspace root files task");

  const sessionData = (await apiClient.listTaskSessions(task.id)) as {
    sessions: Array<{ worktrees?: Array<{ worktree_path?: string }> }>;
  };
  const repoPaths = (sessionData.sessions[0]?.worktrees ?? []).flatMap((worktree) =>
    worktree.worktree_path ? [worktree.worktree_path] : [],
  );
  expect(repoPaths).toHaveLength(2);
  const workspaceRoot = path.dirname(repoPaths[0]);
  expect(repoPaths.map((repoPath) => path.dirname(repoPath))).toEqual([
    workspaceRoot,
    workspaceRoot,
  ]);
  expect(repoPaths.map((repoPath) => gitStatus(repoPath, gitEnv))).toEqual(["", ""]);

  await writeWorkspaceRootFileAndWait({
    watcher,
    sessionId: task.session_id,
    workspaceRoot,
    filePath: "dashboard.json",
    content: `${ROOT_FILE_CONTENT}\n`,
    waitForListing: false,
  });
  return { session, sessionId: task.session_id, repoPaths, gitEnv, workspaceRoot };
}

export function expectCheckoutsClean(repoPaths: string[], gitEnv: NodeJS.ProcessEnv) {
  expect(repoPaths.map((repoPath) => gitStatus(repoPath, gitEnv))).toEqual(["", ""]);
}

export async function expectRootFileUnstagedInRepository(
  repoPaths: string[],
  gitEnv: NodeJS.ProcessEnv,
  repositoryIndex: number,
  filePath: string,
) {
  await expect
    .poll(() => repoPaths.map((repoPath) => gitStatus(repoPath, gitEnv)))
    .toEqual(repoPaths.map((_, index) => (index === repositoryIndex ? `?? ${filePath}` : "")));
}

export function writeWorkspaceRootFile(workspaceRoot: string, filePath: string, content: string) {
  fs.writeFileSync(path.join(workspaceRoot, filePath), content);
}

export async function writeWorkspaceRootFileAndWait({
  watcher,
  sessionId,
  workspaceRoot,
  filePath,
  content,
  waitForListing = true,
}: {
  watcher: WsWatcher;
  sessionId: string;
  workspaceRoot: string;
  filePath: string;
  content: string;
  waitForListing?: boolean;
}) {
  const fileChanged = watcher.waitForEvent("session.workspace.file.changes", {
    where: (payload) => isRootFileChange(payload, sessionId, filePath),
  });
  const listingRefreshed = waitForListing
    ? watcher.waitForResponse("workspace.root_files.list", {
        requestWhere: (payload) => payload.session_id === sessionId,
        timeoutAfter: fileChanged,
      })
    : undefined;
  writeWorkspaceRootFile(workspaceRoot, filePath, content);
  await fileChanged;
  await listingRefreshed;
}

export function expectWorkspaceRootFileContents(
  workspaceRoot: string,
  filePath: string,
  content: string,
) {
  expect(fs.readFileSync(path.join(workspaceRoot, filePath), "utf8")).toBe(content);
}

export async function moveRootFileToSecondRepository(
  testPage: Page,
  rootFiles: Locator,
  input: "click" | "tap",
  {
    watcher,
    sessionId,
    expectSuccess,
  }: { watcher: WsWatcher; sessionId: string; expectSuccess: boolean },
) {
  const activate = async (locator: Locator) => {
    if (input === "tap") await locator.tap();
    else await locator.click();
  };

  await activate(rootFiles.getByRole("button", { name: "Show more actions" }));
  await activate(testPage.getByRole("menuitem", { name: "Move to repository..." }));
  const dialog = testPage.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const destination = dialog.getByRole("radio").nth(1);
  const repositoryName = await destination
    .locator("xpath=ancestor::label")
    .getByTestId("workspace-root-destination-name")
    .innerText();
  await activate(destination);
  const moveButton = dialog.getByRole("button", { name: "Move", exact: true });
  await expect(moveButton).toBeEnabled();

  const renameResponse = watcher
    .waitForResponse("workspace.file.rename", {
      requestWhere: (payload) =>
        payload.session_id === sessionId && payload.old_path === "dashboard.json",
    })
    .then(
      (frame) => ({ frame }),
      (error: unknown) => ({ error }),
    );
  const rootFileChange = expectSuccess
    ? watcher.waitForEvent("session.workspace.file.changes", {
        where: (payload) => isRootFileChange(payload, sessionId, "dashboard.json"),
      })
    : undefined;
  const listingRefresh =
    rootFileChange &&
    watcher.waitForResponse("workspace.root_files.list", {
      requestWhere: (payload) => payload.session_id === sessionId,
      timeoutAfter: rootFileChange,
    });

  await activate(moveButton);
  const outcome = await renameResponse;
  if (expectSuccess) {
    if ("error" in outcome) throw outcome.error;
    expect(outcome.frame.payload).toMatchObject({ success: true });
    await rootFileChange;
    await listingRefresh;
  } else if ("frame" in outcome) {
    expect(outcome.frame.payload).toMatchObject({ success: false });
  } else if (
    !(outcome.error instanceof Error) ||
    !outcome.error.message.startsWith(
      'watchWs.waitForResponse: "workspace.file.rename" was rejected:',
    ) ||
    !outcome.error.message.includes("target already exists")
  ) {
    throw outcome.error;
  }
  return repositoryName.trim();
}

function isRootFileChange(payload: Record<string, unknown>, sessionId: string, filePath: string) {
  if (payload.session_id !== sessionId || !Array.isArray(payload.changes)) return false;
  return payload.changes.some((change) => {
    if (!change || typeof change !== "object") return false;
    const event = change as { path?: unknown; repository_name?: unknown };
    return event.path === filePath && !event.repository_name;
  });
}

function gitStatus(repoPath: string, gitEnv: NodeJS.ProcessEnv) {
  return execSync("git status --porcelain --untracked-files=all", {
    cwd: repoPath,
    env: gitEnv,
    encoding: "utf8",
  }).trim();
}

export const workspaceRootFileMarker = ROOT_FILE_CONTENT;
