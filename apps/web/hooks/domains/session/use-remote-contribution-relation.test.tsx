import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PRCommitInfo, TaskPR } from "@/lib/types/github";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { useRemoteContributionRelation } from "./use-remote-contribution-relation";

const mocks = vi.hoisted(() => ({
  statuses: [] as Array<{ repository_name: string; status: GitStatusEntry }>,
  selectedPR: null as TaskPR | null,
  providerHead: "provider-head",
  repositoryName: "frontend",
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { tasks: { activeTaskId: string } }) => unknown) =>
    selector({ tasks: { activeTaskId: "task-1" } }),
}));

vi.mock("@/hooks/domains/github/use-review-pr-selection", () => ({
  useReviewPRSelection: () => ({ selectedPR: mocks.selectedPR }),
}));

vi.mock("@/hooks/domains/github/use-pr-commits", () => ({
  usePRCommits: () => ({
    commits: [{ sha: mocks.providerHead }] as PRCommitInfo[],
    providerHead: mocks.providerHead,
    providerCommitsComplete: true,
    loading: false,
    error: null,
  }),
}));

vi.mock("@/hooks/domains/github/use-pr-review-repository-identity", () => ({
  usePRReviewRepositoryIdentity: () => mocks.repositoryName,
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

const selectedPR = { repository_id: "repo-frontend" } as TaskPR;

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
});
