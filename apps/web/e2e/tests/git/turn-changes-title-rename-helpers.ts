import { expect, type Page } from "@playwright/test";
import fs from "node:fs";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { closePreviewPanels } from "./diff-update-helpers";
import { readTurnChangeHistory, waitForTurnChangeCount } from "./turn-changed-files-helpers";

export const TITLE_RENAME_FINAL_TITLE = "Renamed turn continuity";
export const TITLE_RENAME_BRANCH_PREFIX = "feature/renamed-turn-continu";
export const TITLE_RENAME_SWITCH_BRANCH = "kandev-title-rename-switch";
export const TITLE_RENAME_FILE = "turn-title-rename.txt";
export const TITLE_RENAME_CONTENT = "renamed turn edit";
export const TITLE_RENAME_LIVE_EDIT = "changed after the retained turn";

export const TITLE_RENAME_SCENARIOS = {
  only: {
    command: "/e2e:turn-title-rename-only",
    completion: "turn-title-rename-only complete",
  },
  edit: {
    command: "/e2e:turn-title-rename-edit",
    completion: "turn-title-rename-edit complete",
  },
  switch: {
    command: "/e2e:turn-title-rename-switch",
    completion: "turn-title-rename-switch complete",
  },
} as const;

export type TitleRenameTask = {
  session: SessionPage;
  sessionId: string;
  taskId: string;
  worktreePath: string;
  worktreeBranch: string;
};

/**
 * Creates a prompt-first task through the real HTTP contract so the mock agent
 * resolves the title with `set_task_title_kandev` during its first turn.
 */
export async function createTitleRenameTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  scenario: (typeof TITLE_RENAME_SCENARIOS)[keyof typeof TITLE_RENAME_SCENARIOS],
): Promise<TitleRenameTask> {
  const response = await apiClient.rawRequest("POST", "/api/v1/tasks", {
    workspace_id: seedData.workspaceId,
    description: scenario.command,
    auto_title: true,
    start_agent: true,
    agent_profile_id: seedData.agentProfileId,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
    repositories: [{ repository_id: seedData.repositoryId }],
  });
  expect(response.ok).toBe(true);
  const task = (await response.json()) as { id: string; session_id?: string };
  expect(task.id).toBeTruthy();
  expect(task.session_id).toBeTruthy();

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await closePreviewPanels(testPage);
  await expect(session.chat.getByText(scenario.completion, { exact: false })).toBeVisible({
    timeout: 90_000,
  });
  await session.waitForChatIdle();
  await waitForTurnChangeCount(apiClient, task.session_id!, 1);

  const environment = await apiClient.getTaskEnvironment(task.id);
  const repository = environment?.repos?.[0];
  expect(repository?.worktree_path).toBeTruthy();
  expect(repository?.worktree_branch).toBeTruthy();
  return {
    session,
    sessionId: task.session_id!,
    taskId: task.id,
    worktreePath: repository!.worktree_path!,
    worktreeBranch: repository!.worktree_branch!,
  };
}

export async function latestTitleRenameChangeSet(apiClient: ApiClient, sessionId: string) {
  const history = await readTurnChangeHistory(apiClient, sessionId);
  const changeSets = [...(history.change_sets ?? [])].sort(
    (left, right) => right.turn_ordinal - left.turn_ordinal,
  );
  expect(changeSets.length).toBeGreaterThan(0);
  return { history, changeSet: changeSets[0]! };
}

export async function expectReadyRenameOnlyComparison(apiClient: ApiClient, sessionId: string) {
  const { changeSet } = await latestTitleRenameChangeSet(apiClient, sessionId);
  expect(changeSet.terminal_at ?? changeSet.terminal_outcome).toBeTruthy();
  expect(changeSet.availability).toBe("ready");
  expect(changeSet.file_count).toBe(0);
  expect(changeSet.repositories?.[0]?.availability).toBe("ready");
  expect(changeSet.repositories?.[0]?.comparison_basis).toBe("direct");
  return changeSet;
}

export async function expectReadyRenameFileComparison(apiClient: ApiClient, sessionId: string) {
  const { changeSet } = await latestTitleRenameChangeSet(apiClient, sessionId);
  expect(changeSet.terminal_at ?? changeSet.terminal_outcome).toBeTruthy();
  expect(changeSet.availability).toBe("ready");
  expect(changeSet.file_count).toBe(1);
  expect(changeSet.repositories?.[0]?.comparison_basis).toBe("direct");
  return changeSet;
}

export async function expectUnavailableRenameSwitch(apiClient: ApiClient, sessionId: string) {
  const { changeSet } = await latestTitleRenameChangeSet(apiClient, sessionId);
  expect(changeSet.terminal_at ?? changeSet.terminal_outcome).toBeTruthy();
  expect(changeSet.availability).toBe("unavailable");
  expect(changeSet.reason).toBe("history_context_changed");
  expect(changeSet.file_count).toBe(0);
  expect(changeSet.repositories?.[0]?.comparison_basis).toBe("history_unavailable");
  return changeSet;
}

export async function assertNoTurnChangesCard(session: SessionPage) {
  await expect(session.activeChat().getByTestId("turn-changed-files-card")).toHaveCount(0);
}

export function writeLiveWorktreeEdit(worktreePath: string) {
  fs.writeFileSync(`${worktreePath}/${TITLE_RENAME_FILE}`, `${TITLE_RENAME_LIVE_EDIT}\n`);
}
