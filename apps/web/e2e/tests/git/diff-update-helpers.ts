import { expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import type { Page } from "@playwright/test";

const DIFFS_CONTAINER = "diffs-container";

type SeedTaskOptions = {
  title: string;
  seedWorkspace: (workspacePath: string) => void;
};

type SeededDiffTask = {
  session: SessionPage;
  sessionId: string;
  workspacePath: string;
};

async function seedTaskWithScenario(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  options: SeedTaskOptions,
): Promise<SeededDiffTask> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    options.title,
    seedData.agentProfileId,
    {
      description: "Prepare git diff fixture",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  const workspacePath = await waitForWorkspacePath(apiClient, task.id);
  options.seedWorkspace(workspacePath);

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await closePreviewPanels(testPage);

  return { session, sessionId: task.session_id, workspacePath };
}

async function waitForWorkspacePath(apiClient: ApiClient, taskId: string) {
  let workspacePath: string | null = null;
  await expect
    .poll(
      async () => {
        const env = await apiClient.getTaskEnvironment(taskId);
        if (env?.status !== "ready") return null;
        workspacePath = env.worktree_path ?? env.workspace_path ?? null;
        return workspacePath;
      },
      { timeout: 45_000, message: "task workspace path should be available" },
    )
    .not.toBeNull();
  if (!workspacePath) throw new Error("task environment did not expose a workspace path");
  return workspacePath;
}

function gitForWorkspace(workspacePath: string) {
  return new GitHelper(workspacePath, makeGitEnv(workspacePath));
}

function commitPathsIfChanged(git: GitHelper, paths: string[], message: string) {
  const quotedPaths = paths.map((filePath) => `"${filePath}"`).join(" ");
  if (git.exec(`git status --porcelain -- ${quotedPaths}`).trim() === "") return;
  git.commit(message);
}

function seedDiffUpdateWorkspace(workspacePath: string) {
  const git = gitForWorkspace(workspacePath);
  const filePath = "diff_update_test.txt";
  const originalContent = "line 1: original\nline 2: unchanged\nline 3: original\n";
  const modifiedContent = "line 1: FIRST_MODIFICATION\nline 2: unchanged\nline 3: original\n";

  git.exec(`git rm --force --ignore-unmatch "${filePath}"`);
  commitPathsIfChanged(git, [filePath], "cleanup diff_update_test.txt");

  git.createFile(filePath, originalContent);
  git.stageFile(filePath);
  commitPathsIfChanged(git, [filePath], "add diff_update_test.txt");

  git.modifyFile(filePath, modifiedContent);
}

function seedMultiFileWorkspace(workspacePath: string) {
  const git = gitForWorkspace(workspacePath);
  const files = ["multi_a.txt", "multi_b.txt", "multi_c.txt"];

  for (const filePath of files) {
    git.exec(`git rm --force --ignore-unmatch "${filePath}"`);
  }
  commitPathsIfChanged(git, files, "cleanup multi-file fixtures");

  for (const filePath of files) {
    const original = `${filePath} line 1: original\n${filePath} line 2: unchanged\n${filePath} line 3: original\n`;
    git.createFile(filePath, original);
    git.stageFile(filePath);
  }
  commitPathsIfChanged(git, files, "add multi-file fixtures");

  for (const filePath of files) {
    const modified = `${filePath} line 1: FIRST_MODIFICATION\n${filePath} line 2: unchanged\n${filePath} line 3: original\n`;
    git.modifyFile(filePath, modified);
  }
}

function seedUntrackedWorkspace(workspacePath: string) {
  const git = gitForWorkspace(workspacePath);
  const filePath = "untracked_test.txt";

  git.exec(`git rm --force --ignore-unmatch "${filePath}"`);
  commitPathsIfChanged(git, [filePath], "cleanup untracked fixture");
  git.deleteFile(filePath);
  git.createFile(filePath, "line 1: INITIAL_CONTENT\nline 2: some text\n");
}

export function writeDiffUpdateSecondModification(workspacePath: string) {
  gitForWorkspace(workspacePath).modifyFile(
    "diff_update_test.txt",
    "line 1: SECOND_MODIFICATION\nline 2: unchanged\nline 3: ALSO_CHANGED\n",
  );
}

export function writeMultiFileSecondModification(workspacePath: string) {
  const git = gitForWorkspace(workspacePath);
  for (const [index, filePath] of ["multi_a.txt", "multi_b.txt", "multi_c.txt"].entries()) {
    git.modifyFile(
      filePath,
      `${filePath} line 1: SECOND_MODIFICATION\n${filePath} line 2: unchanged\n${filePath} line 3: ALSO_CHANGED_${index}\n`,
    );
  }
}

export function writeUntrackedModification(workspacePath: string) {
  gitForWorkspace(workspacePath).modifyFile(
    "untracked_test.txt",
    "line 1: MODIFIED_CONTENT\nline 2: some text\nline 3: NEW_LINE\n",
  );
}

export async function closePreviewPanels(testPage: Page) {
  await expect
    .poll(
      () =>
        testPage.evaluate(() => {
          type TestWindow = Window & {
            __dockviewApi__?: {
              getPanel: (id: string) => unknown;
              removePanel: (panel: unknown) => void;
            };
          };
          const dockview = (window as TestWindow).__dockviewApi__;
          if (!dockview) return false;
          for (const id of ["preview:file-diff", "preview:file-editor"]) {
            const panel = dockview.getPanel(id);
            if (panel) dockview.removePanel(panel);
          }
          return true;
        }),
      { timeout: 10_000 },
    )
    .toBe(true);
}

export function seedUntrackedFileTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  return seedTaskWithScenario(testPage, apiClient, seedData, {
    title: "Untracked File E2E",
    seedWorkspace: seedUntrackedWorkspace,
  });
}

export function seedDiffUpdateTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  return seedTaskWithScenario(testPage, apiClient, seedData, {
    title: "Diff Update E2E",
    seedWorkspace: seedDiffUpdateWorkspace,
  });
}

export function seedMultiFileTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  return seedTaskWithScenario(testPage, apiClient, seedData, {
    title: "Multi-file Diff Update E2E",
    seedWorkspace: seedMultiFileWorkspace,
  });
}

export async function openChangesTab(testPage: Page) {
  const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
  await expect(changesTab).toBeVisible({ timeout: 10_000 });
  await changesTab.click();
}

export async function openFileDiff(testPage: Page, fileName: string) {
  const fileRow = testPage.getByTestId(`file-row-${fileName.replace(/[/\\]/g, "-")}`);
  await expect(fileRow).toBeVisible({ timeout: 10_000 });
  await fileRow.click();
}

export function fileDiffTab(testPage: Page, fileName: string) {
  return testPage.locator(".dv-default-tab[type='file-diff']", {
    hasText: `Diff [${fileName}]`,
  });
}

export function getDiffsContainer(testPage: Page) {
  return testPage.locator(DIFFS_CONTAINER);
}

export async function waitForDiffText(testPage: Page, text: string, timeout = 60_000) {
  await expect
    .poll(
      () =>
        testPage.evaluate((expected) => {
          const containers = Array.from(document.querySelectorAll("diffs-container")).filter(
            (el) => {
              const rect = el.getBoundingClientRect();
              const style = window.getComputedStyle(el);
              return (
                rect.width > 0 &&
                rect.height > 0 &&
                style.display !== "none" &&
                style.visibility !== "hidden"
              );
            },
          );
          return containers.some((container) =>
            container.shadowRoot?.textContent?.includes(expected),
          );
        }, text),
      { timeout },
    )
    .toBe(true);
}

export async function waitForDiffTextAbsent(testPage: Page, text: string, timeout = 5_000) {
  await expect
    .poll(
      () =>
        testPage.evaluate((expected) => {
          const containers = Array.from(document.querySelectorAll("diffs-container")).filter(
            (el) => {
              const rect = el.getBoundingClientRect();
              const style = window.getComputedStyle(el);
              return (
                rect.width > 0 &&
                rect.height > 0 &&
                style.display !== "none" &&
                style.visibility !== "hidden"
              );
            },
          );
          return containers.every(
            (container) => !container.shadowRoot?.textContent?.includes(expected),
          );
        }, text),
      { timeout },
    )
    .toBe(true);
}

export async function waitForStoreFileDiffText(
  testPage: Page,
  sessionId: string,
  filePath: string,
  text: string,
  timeout = 30_000,
) {
  await expect
    .poll(
      () =>
        testPage.evaluate(
          ({ sid, path, expected }) => {
            type E2EStoreWindow = Window & {
              __KANDEV_E2E_STORE__?: {
                getState: () => {
                  environmentIdBySessionId: Record<string, string>;
                  gitStatus: {
                    byEnvironmentId: Record<
                      string,
                      { files?: Record<string, { diff?: string }> } | undefined
                    >;
                  };
                };
              };
            };
            const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
            const state = store?.getState();
            if (!state) return false;
            const envKey = state.environmentIdBySessionId[sid] ?? sid;
            return state.gitStatus.byEnvironmentId[envKey]?.files?.[path]?.diff?.includes(expected);
          },
          { sid: sessionId, path: filePath, expected: text },
        ),
      { timeout },
    )
    .toBe(true);
}
