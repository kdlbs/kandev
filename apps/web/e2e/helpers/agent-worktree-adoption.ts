import { expect, type Page } from "@playwright/test";
import type { GitStatusData } from "../../lib/types/git-events";
import type { ApiClient } from "./api-client";
import { waitForHttp, type WsWatcher } from "./causal-waits";

export const AGENT_WORKTREE_FILE = "worktree-change.txt";
export const AGENT_WORKTREE_BRANCH = "fix-b";
export const AGENT_WORKTREE_TRACKER = `.claude/worktrees/${AGENT_WORKTREE_BRANCH}`;

export function agentWorktreePrompt(reported: boolean): string {
  return `/e2e:agent-worktree-${reported ? "reported" : "unreported"} ${JSON.stringify({
    branch: AGENT_WORKTREE_BRANCH,
  })}`;
}

export function agentWorktreeGroup(page: Page) {
  return page.locator(
    `[data-testid="changes-repo-group"][data-repository-name="${AGENT_WORKTREE_TRACKER}"]:visible`,
  );
}

/** Wait for the backend to persist the adopted root before reading the UI. */
export async function expectAgentWorktreeAttached(apiClient: ApiClient, taskId: string) {
  await expect
    .poll(
      async () => {
        const task = await apiClient.getTask(taskId);
        return (task.repositories ?? []).some(
          (repository) => repository.workspace_relative_path === AGENT_WORKTREE_TRACKER,
        );
      },
      { message: `agent worktree ${AGENT_WORKTREE_TRACKER} was not attached`, timeout: 60_000 },
    )
    .toBe(true);
}

function statusHasAgentWorktree(status: GitStatusData | undefined): boolean {
  return (
    status?.repository_name === AGENT_WORKTREE_TRACKER &&
    Boolean(status.files?.[AGENT_WORKTREE_FILE])
  );
}

/** Wait for the pushed git status that renders a newly adopted root. */
export function waitForAgentWorktreeStatus(watcher: WsWatcher, sessionId: string) {
  return watcher.waitForEvent("session.git.event", {
    where: (payload) =>
      payload.session_id === sessionId &&
      payload.type === "status_update" &&
      statusHasAgentWorktree(payload.status as GitStatusData | undefined),
  });
}

/** Wait for the task update that publishes the adopted repository list. */
export function waitForAgentWorktreeTaskUpdate(watcher: WsWatcher, taskId: string) {
  return watcher.waitForEvent("task.updated", {
    where: (payload) =>
      payload.task_id === taskId &&
      Array.isArray(payload.repositories) &&
      (payload.repositories as Array<{ workspace_relative_path?: string }>).some(
        (repository) => repository.workspace_relative_path === AGENT_WORKTREE_TRACKER,
      ),
  });
}

export async function expectAgentWorktreeGroup(page: Page) {
  const group = agentWorktreeGroup(page);
  await expect(group).toHaveCount(1);
  await expect(group.getByTestId("changes-repo-header")).toBeVisible();
  return group;
}

/** Confirm removal, await the detach response, then assert the UI caught up. */
export async function confirmAgentWorktreeRemoval(page: Page) {
  const detached = waitForHttp(page, "DELETE", /\/tasks\/[^/]+\/repositories\/[^/]+$/, {
    predicate: (response) => response.ok(),
  });
  await page.getByTestId("remove-root-confirm").click();
  await detached;
  await expect(agentWorktreeGroup(page)).toHaveCount(0);
}

export async function removeAgentWorktreeRoot(page: Page) {
  const group = await expectAgentWorktreeGroup(page);
  await group.getByTestId("changes-repo-header").click({ button: "right" });
  await page.getByTestId("root-menu-remove").click();
  await confirmAgentWorktreeRemoval(page);
}
