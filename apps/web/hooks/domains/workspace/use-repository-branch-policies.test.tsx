import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";

import { StateProvider, useAppStore } from "@/components/state-provider";
import type { RepositoryBranchPolicy } from "@/lib/types/http";
import type { HydrationState } from "@/lib/state/store";

const { createPolicyMock, updatePolicyMock, seedGitflowMock, listPoliciesMock } = vi.hoisted(
  () => ({
    createPolicyMock: vi.fn(),
    updatePolicyMock: vi.fn(),
    seedGitflowMock: vi.fn(),
    listPoliciesMock: vi.fn(),
  }),
);

vi.mock("@/lib/api", () => ({
  createRepositoryBranchPolicy: createPolicyMock,
  updateRepositoryBranchPolicy: updatePolicyMock,
  createGitflowRepositoryBranchPolicies: seedGitflowMock,
  listRepositoryBranchPolicies: listPoliciesMock,
  deleteRepositoryBranchPolicy: vi.fn(),
}));

import { useRepositoryBranchPolicies } from "./use-repository-branch-policies";

const REPOSITORY_ID = "repo-1";
const EMPTY_POLICIES: RepositoryBranchPolicy[] = [];

function policy(id: string, name: string): RepositoryBranchPolicy {
  return {
    id,
    repository_id: REPOSITORY_ID as RepositoryBranchPolicy["repository_id"],
    name,
    description: "",
    base_branch: "main",
    branch_template: "feature/{title}-{suffix}",
    pull_request_target: "main",
    created_at: "2026-08-24T10:00:00Z",
    updated_at: "2026-08-24T10:00:00Z",
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

function initialState(): HydrationState {
  return {
    repositoryBranchPolicies: {
      itemsByRepositoryId: { [REPOSITORY_ID]: [] },
      loadingByRepositoryId: { [REPOSITORY_ID]: false },
      loadedByRepositoryId: { [REPOSITORY_ID]: true },
      revisionByRepositoryId: { [REPOSITORY_ID]: 0 },
    },
  };
}

function useHarness() {
  const branchPolicies = useRepositoryBranchPolicies(REPOSITORY_ID);
  const policiesByRepositoryId = useAppStore(
    (state) => state.repositoryBranchPolicies.itemsByRepositoryId,
  );
  const policies = policiesByRepositoryId[REPOSITORY_ID] ?? EMPTY_POLICIES;
  const upsert = useAppStore((state) => state.upsertRepositoryBranchPolicy);
  return { branchPolicies, policies, upsert };
}

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider initialState={initialState()}>{children}</StateProvider>;
}

describe("useRepositoryBranchPolicies mutation races", () => {
  it("surfaces an initial policy list failure instead of treating it as empty", async () => {
    const failedState = initialState();
    failedState.repositoryBranchPolicies!.loadedByRepositoryId[REPOSITORY_ID] = false;
    listPoliciesMock.mockRejectedValueOnce(new Error("temporary failure"));

    const failedWrapper = ({ children }: { children: ReactNode }) => (
      <StateProvider initialState={failedState}>{children}</StateProvider>
    );
    const { result } = renderHook(() => useRepositoryBranchPolicies(REPOSITORY_ID), {
      wrapper: failedWrapper,
    });

    await waitFor(() => expect(result.current.hasError).toBe(true));
    expect(result.current.isLoading).toBe(false);
    expect(result.current.policies).toEqual([]);
  });

  it("refreshes instead of applying an older create response after a WS update", async () => {
    const response = deferred<RepositoryBranchPolicy>();
    const current = policy("current", "Current policy");
    const stale = policy("stale", "Stale response");
    createPolicyMock.mockReturnValueOnce(response.promise);
    listPoliciesMock.mockResolvedValue({ repository_branch_policies: [current] });

    const { result } = renderHook(() => useHarness(), { wrapper });
    let pending: Promise<RepositoryBranchPolicy> | undefined;
    act(() => {
      pending = result.current.branchPolicies.create({
        name: stale.name,
        description: stale.description,
        base_branch: stale.base_branch,
        branch_template: stale.branch_template,
        pull_request_target: stale.pull_request_target,
      });
      result.current.upsert(current);
    });
    await act(async () => {
      response.resolve(stale);
      await pending;
    });

    await waitFor(() => expect(result.current.policies).toEqual([current]));
    expect(listPoliciesMock).toHaveBeenCalledWith(REPOSITORY_ID, { cache: "no-store" });
  });

  it("refreshes instead of applying an older update response after a WS update", async () => {
    const response = deferred<RepositoryBranchPolicy>();
    const current = policy("current", "Current policy");
    const stale = policy("stale", "Stale response");
    updatePolicyMock.mockReturnValueOnce(response.promise);
    listPoliciesMock.mockResolvedValue({ repository_branch_policies: [current] });

    const { result } = renderHook(() => useHarness(), { wrapper });
    let pending: Promise<RepositoryBranchPolicy> | undefined;
    act(() => {
      pending = result.current.branchPolicies.update(stale.id, {
        name: stale.name,
      });
      result.current.upsert(current);
    });
    await act(async () => {
      response.resolve(stale);
      await pending;
    });

    await waitFor(() => expect(result.current.policies).toEqual([current]));
    expect(listPoliciesMock).toHaveBeenCalledWith(REPOSITORY_ID, { cache: "no-store" });
  });

  it("guards Gitflow seeding against a newer WS policy update", async () => {
    const response = deferred<{ repository_branch_policies: RepositoryBranchPolicy[] }>();
    const current = policy("current", "Current policy");
    const stale = policy("stale", "Stale Gitflow policy");
    seedGitflowMock.mockReturnValueOnce(response.promise);
    listPoliciesMock.mockResolvedValue({ repository_branch_policies: [current] });

    const { result } = renderHook(() => useHarness(), { wrapper });
    let pending: Promise<RepositoryBranchPolicy[]> | undefined;
    act(() => {
      pending = result.current.branchPolicies.seedGitflow("main", "develop");
      result.current.upsert(current);
    });
    await act(async () => {
      response.resolve({ repository_branch_policies: [stale] });
      await pending;
    });

    await waitFor(() => expect(result.current.policies).toEqual([current]));
    expect(listPoliciesMock).toHaveBeenCalledWith(REPOSITORY_ID, { cache: "no-store" });
  });
});
