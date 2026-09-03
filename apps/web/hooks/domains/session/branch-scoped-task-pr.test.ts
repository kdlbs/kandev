import { describe, expect, it } from "vitest";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import type { TaskPR } from "@/lib/types/github";
import {
  normalizeContributionBranch,
  resolveBranchScopedTaskPRs,
  selectBranchScopedTaskPR,
} from "./branch-scoped-task-pr";

const CURRENT_BRANCH = "feature/current";

function taskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-2",
    workspace_id: "workspace-1",
    task_id: "task-1",
    repository_id: "repo-frontend",
    owner: "acme",
    repo: "frontend",
    pr_number: 2,
    pr_url: "https://github.com/acme/frontend/pull/2",
    pr_title: "Current contribution",
    head_branch: CURRENT_BRANCH,
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "2026-08-12T10:00:00Z",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "2026-08-12T10:00:00Z",
    ...overrides,
  };
}

function gitStatus(branch: string): GitStatusEntry {
  return {
    branch,
    remote_branch: `origin/${branch}`,
    modified: [],
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files: {},
    timestamp: null,
  };
}

describe("branch-scoped task PR selection", () => {
  it("keeps a merged historical branch out of the live checkout relation", () => {
    const historical = taskPR({
      id: "pr-1",
      pr_number: 1,
      head_branch: "feature/merged",
      state: "merged",
    });
    const current = taskPR();

    const scoped = resolveBranchScopedTaskPRs({
      prs: [historical, current],
      statuses: [{ repository_name: "frontend", status: gitStatus(CURRENT_BRANCH) }],
      preferredKey: "acme/frontend/1",
      resolveRepositoryName: () => "frontend",
    });

    expect(scoped.map((entry) => entry.pr)).toEqual([current]);
    expect(selectBranchScopedTaskPR(scoped, "acme/frontend/1")?.pr).toEqual(current);
  });

  it("prefers an open PR over a terminal sibling on the same branch", () => {
    const merged = taskPR({ id: "pr-1", pr_number: 1, state: "merged" });
    const current = taskPR();

    const scoped = resolveBranchScopedTaskPRs({
      prs: [merged, current],
      statuses: [{ repository_name: "frontend", status: gitStatus(CURRENT_BRANCH) }],
      preferredKey: "acme/frontend/1",
      resolveRepositoryName: () => "frontend",
    });

    expect(scoped.map((entry) => entry.pr)).toEqual([current]);
  });

  it("requires repository identity when multiple checkouts use the same branch", () => {
    const frontend = taskPR();
    const backend = taskPR({
      id: "pr-backend",
      repository_id: "repo-backend",
      repo: "backend",
      pr_number: 3,
      pr_url: "https://github.com/acme/backend/pull/3",
    });

    const scoped = resolveBranchScopedTaskPRs({
      prs: [frontend, backend],
      statuses: [
        { repository_name: "backend", status: gitStatus(CURRENT_BRANCH) },
        { repository_name: "frontend", status: gitStatus(CURRENT_BRANCH) },
      ],
      resolveRepositoryName: (pr) => (pr.repository_id === "repo-backend" ? "backend" : "frontend"),
    });

    expect(scoped.map((entry) => entry.pr.id)).toEqual(["pr-backend", "pr-2"]);
  });

  it("normalizes full local and remote branch refs", () => {
    expect(normalizeContributionBranch(`refs/heads/${CURRENT_BRANCH}`)).toBe(CURRENT_BRANCH);
    expect(normalizeContributionBranch(`refs/remotes/origin/${CURRENT_BRANCH}`)).toBe(
      CURRENT_BRANCH,
    );
  });
});
