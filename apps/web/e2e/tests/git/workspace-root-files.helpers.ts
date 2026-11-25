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
export const WORKSPACE_ROOT_NESTED_FILE_PATH = "reports/daily/summary.md";
export const WORKSPACE_ROOT_NESTED_FILE_CONTENT = "nested-workspace-root-file-marker";
export const DEEP_TREE_LEVEL_COUNT = 32;

export function deepRootDirectoryPath(level: number) {
  return ["deep", ...deepTreeSegments(level)].join("/");
}

export function deepRootFilePath(level: number) {
  const levelLabel = String(level).padStart(2, "0");
  return `${deepRootDirectoryPath(level)}/file-${levelLabel}.txt`;
}

export function deepRootFileContent(level: number) {
  return `deep-workspace-root-file-marker-${level}`;
}

export async function createWorkspaceRootFilesFixture({
  testPage,
  apiClient,
  seedData,
  backend,
  watcher,
  includeDeepTree = false,
}: {
  testPage: import("@playwright/test").Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  watcher: WsWatcher;
  includeDeepTree?: boolean;
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

  if (includeDeepTree) {
    for (let level = 1; level <= DEEP_TREE_LEVEL_COUNT; level++) {
      writeWorkspaceRootFile(
        workspaceRoot,
        deepRootFilePath(level),
        `${deepRootFileContent(level)}\n`,
      );
    }
  }

  await writeWorkspaceRootFileAndWait({
    watcher,
    sessionId: task.session_id,
    workspaceRoot,
    filePath: "dashboard.json",
    content: `${ROOT_FILE_CONTENT}\n`,
    waitForListing: false,
  });
  await writeWorkspaceRootFileAndWait({
    watcher,
    sessionId: task.session_id,
    workspaceRoot,
    filePath: WORKSPACE_ROOT_NESTED_FILE_PATH,
    content: `${WORKSPACE_ROOT_NESTED_FILE_CONTENT}\n`,
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
  const absolutePath = path.join(workspaceRoot, filePath);
  fs.mkdirSync(path.dirname(absolutePath), { recursive: true });
  fs.writeFileSync(absolutePath, content);
}

export async function setChangesPanelLayout(page: Page, layout: "flat" | "tree") {
  await page.evaluate((selectedLayout) => {
    type E2EState = {
      userSettings: { changesPanelLayout: "flat" | "tree"; [key: string]: unknown };
      setUserSettings: (settings: E2EState["userSettings"]) => void;
    };
    const store = (window as Window & { __KANDEV_E2E_STORE__?: { getState: () => E2EState } })
      .__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge missing");
    const state = store.getState();
    state.setUserSettings({ ...state.userSettings, changesPanelLayout: selectedLayout });
  }, layout);
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
  filePath: string,
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

  await activate(
    rootFiles
      .getByTestId(`workspace-root-file-row-${filePath.replaceAll("/", "-")}`)
      .getByRole("button", { name: "Show more actions" }),
  );
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
      requestWhere: (payload) => payload.session_id === sessionId && payload.old_path === filePath,
    })
    .then(
      (frame) => ({ frame }),
      (error: unknown) => ({ error }),
    );
  const rootFileChange = expectSuccess
    ? watcher.waitForEvent("session.workspace.file.changes", {
        where: (payload) => isRootFileChange(payload, sessionId, filePath),
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

function deepTreeSegments(level: number) {
  return Array.from({ length: level }, (_, index) => `level-${String(index + 1).padStart(2, "0")}`);
}

export const workspaceRootFileMarker = ROOT_FILE_CONTENT;
