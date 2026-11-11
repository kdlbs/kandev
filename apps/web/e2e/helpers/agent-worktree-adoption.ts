import { expect, type Page } from "@playwright/test";

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

export async function expectAgentWorktreeGroup(page: Page, timeout = 60_000) {
  const group = agentWorktreeGroup(page);
  await expect(group).toHaveCount(1, { timeout });
  await expect(group.getByTestId("changes-repo-header")).toBeVisible({ timeout });
  return group;
}

export async function removeAgentWorktreeRoot(page: Page) {
  const group = await expectAgentWorktreeGroup(page);
  await group.getByTestId("changes-repo-header").click({ button: "right" });
  await page.getByTestId("root-menu-remove").click();
  await page.getByTestId("remove-root-confirm").click();
  await expect(agentWorktreeGroup(page)).toHaveCount(0, { timeout: 30_000 });
}
