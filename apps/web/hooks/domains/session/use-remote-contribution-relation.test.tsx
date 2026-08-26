import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PRCommitInfo, TaskPR } from "@/lib/types/github";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { useRemoteContributionRelation } from "./use-remote-contribution-relation";

const mocks = vi.hoisted(() => ({
  statuses: [] as Array<{ repository_name: string; status: GitStatusEntry }>,
  prs: [] as TaskPR[],
  selectedPR: null as TaskPR | null,
  selectedKey: null as string | null,
  providerHead: "provider-head",
  repositoryName: "frontend",
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { tasks: { activeTaskId: string } }) => unknown) =>
    selector({ tasks: { activeTaskId: "task-1" } }),
}));

vi.mock("@/hooks/domains/github/use-review-pr-selection", () => ({
  useReviewPRSelection: () => ({
    prs: mocks.prs,
    selectedPR: mocks.selectedPR,
    selectedKey: mocks.selectedKey,
  }),
}));

vi.mock("@/hooks/domains/github/use-pr-commits", () => ({
  usePRCommits: () => ({
    commits: [{ sha: mocks.providerHead }] as PRCommitInfo[],
    providerHead: mocks.providerHead,
    providerCommitsComplete: true,
    loading: false,
    error: null,
    refresh: vi.fn(),
  }),
}));

vi.mock("@/hooks/domains/github/use-pr-review-repository-identity", () => ({
  usePRReviewRepositoryIdentity: () => mocks.repositoryName,
  usePRReviewRepositoryIdentityResolver: () => () => mocks.repositoryName,
}));

vi.mock("./use-session-git-status", () => ({
  useSessionGitStatus: () => mocks.statuses.at(-1)?.status,
  useSessionGitStatusByRepo: () => mocks.statuses,
}));

const baseStatus: Omit<GitStatusEntry, "repository_name" | "head_commit" | "remote_head_commit"> = {
  branch: "feature",
  remote_branch: "origin/feature",
  modified: [],
  added: [],
  deleted: [],
  untracked: [],
  renamed: [],
  ahead: 0,
  behind: 0,
  remote_ahead: 0,
  remote_behind: 0,
  files: {},
  timestamp: null,
};

function taskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-current",
    workspace_id: "workspace-1",
    task_id: "task-1",
    repository_id: "repo-frontend",
    owner: "acme",
    repo: "frontend",
    pr_number: 2,
    pr_url: "https://github.com/acme/frontend/pull/2",
    pr_title: "Current contribution",
    head_branch: "feature",
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

const selectedPR = taskPR();

function status(
  repository_name: string,
  head_commit: string,
  remote_head_commit: string,
  overrides: Partial<GitStatusEntry> = {},
): { repository_name: string; status: GitStatusEntry } {
  return {
    repository_name,
    status: {
      ...baseStatus,
      repository_name,
      head_commit,
      remote_head_commit,
      ...overrides,
    },
  };
}

describe("useRemoteContributionRelation repository scoping", () => {
  beforeEach(() => {
    mocks.selectedPR = selectedPR;
    mocks.prs = [selectedPR];
    mocks.selectedKey = null;
    mocks.statuses = [];
    mocks.repositoryName = "frontend";
  });

  it.each([
    ["frontend then backend", ["frontend", "backend"]],
    ["backend then frontend", ["backend", "frontend"]],
  ])("uses the selected PR repository when statuses arrive %s", (_name, eventOrder) => {
    const statuses = {
      frontend: status("frontend", mocks.providerHead, mocks.providerHead),
      backend: status("backend", "backend-local", "backend-provider", {
        remote_ahead: 3,
        remote_behind: 3,
      }),
    };
    mocks.statuses = eventOrder.map(
      (repositoryName) => statuses[repositoryName as keyof typeof statuses],
    );

    const { result } = renderHook(() => useRemoteContributionRelation("session-1"));

    expect(result.current.relation).toMatchObject({
      kind: "aligned",
      canPush: false,
      canPull: false,
      canReplaceRemote: false,
      canUseRemote: false,
    });
  });

  it("uses the empty-key status for a single-repository session", () => {
    mocks.statuses = [status("", mocks.providerHead, mocks.providerHead)];

    const { result } = renderHook(() => useRemoteContributionRelation("session-1"));

    expect(result.current.relation).toMatchObject({
      kind: "aligned",
      canPush: false,
      canPull: false,
      canReplaceRemote: false,
      canUseRemote: false,
    });
  });

  it("ignores a merged Review PR when a newer PR matches the checked-out branch", () => {
    const historicalPR = taskPR({
      id: "pr-historical",
      pr_number: 1,
      head_branch: "feature/merged",
      state: "merged",
      merged_at: "2026-08-12T09:00:00Z",
      updated_at: "2026-08-12T09:00:00Z",
    });
    const currentPR = taskPR({
      id: "pr-current",
      pr_number: 2,
      head_branch: "feature/current",
    });
    mocks.prs = [historicalPR, currentPR];
    mocks.selectedPR = historicalPR;
    mocks.selectedKey = "acme/frontend/1";
    mocks.statuses = [
      status("", mocks.providerHead, mocks.providerHead, { branch: "feature/current" }),
    ];

    const { result } = renderHook(() => useRemoteContributionRelation("session-1"));

    expect(result.current.selectedPR).toEqual(currentPR);
    expect(result.current.relation.kind).toBe("aligned");
  });
});
