import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { GitOperationResult } from "@/hooks/use-git-operations";
import type { RemoteContributionRelation } from "@/hooks/domains/session/remote-contribution-relation";
import type { TaskPR } from "@/lib/types/github";
import {
  buildRemoteContributionResolutionTarget,
  classifyRemoteContributionError,
  useRemoteContributionResolution,
} from "./use-remote-contribution-resolution";

const mocks = vi.hoisted(() => ({
  replaceRemoteContribution: vi.fn(),
  useRemoteContribution: vi.fn(),
  refreshProviderEvidence: vi.fn(),
}));

vi.mock("@/hooks/use-git-operations", () => ({
  useGitOperations: () => ({
    replaceRemoteContribution: mocks.replaceRemoteContribution,
    useRemoteContribution: mocks.useRemoteContribution,
  }),
}));

const TARGET = {
  expectedRemoteHead: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  repo: "",
  repositoryName: "workspace",
};

const successResult: GitOperationResult = {
  success: true,
  operation: "use_remote_contribution",
  output: "",
  recovery_branch: "kandev/recovery-123",
};

const relationWithProviderHead = {
  providerHead: TARGET.expectedRemoteHead,
  canReplaceRemote: true,
  canUseRemote: true,
} as RemoteContributionRelation;

describe("useRemoteContributionResolution", () => {
  beforeEach(() => {
    mocks.replaceRemoteContribution.mockReset();
    mocks.useRemoteContribution.mockReset();
    mocks.refreshProviderEvidence.mockReset();
    mocks.replaceRemoteContribution.mockResolvedValue({
      ...successResult,
      operation: "replace_remote_contribution",
    });
    mocks.useRemoteContribution.mockResolvedValue(successResult);
    mocks.refreshProviderEvidence.mockResolvedValue(TARGET.expectedRemoteHead);
  });

  it("keeps a typed replacement confirmation pending until it is confirmed", async () => {
    const { result } = renderHook(() =>
      useRemoteContributionResolution("session-1", mocks.refreshProviderEvidence),
    );

    act(() => result.current.requestReplace(TARGET));
    expect(result.current.pending).toEqual({ action: "replace", ...TARGET });

    await act(async () => {
      await result.current.confirm();
    });

    expect(mocks.replaceRemoteContribution).toHaveBeenCalledWith(TARGET.expectedRemoteHead, "");
    expect(mocks.refreshProviderEvidence).toHaveBeenCalledOnce();
    expect(result.current.pending).toBeNull();
    expect(result.current.lastResult?.operation).toBe("replace_remote_contribution");
  });

  it("uses one scoped adoption operation and exposes its recovery branch", async () => {
    const { result } = renderHook(() => useRemoteContributionResolution("session-1"));

    act(() => result.current.requestUse(TARGET));
    await act(async () => {
      await result.current.confirm();
    });

    expect(mocks.useRemoteContribution).toHaveBeenCalledWith(TARGET.expectedRemoteHead, "");
    expect(result.current.lastResult?.recovery_branch).toBe("kandev/recovery-123");
  });

  it("refreshes the provider head after a lease mismatch before retry", async () => {
    const refreshedHead = "cccccccccccccccccccccccccccccccccccccccc";
    mocks.replaceRemoteContribution.mockResolvedValue({
      ...successResult,
      success: false,
      operation: "replace_remote_contribution",
      error: "remote contribution head changed",
      error_code: "lease_mismatch",
    });
    mocks.refreshProviderEvidence.mockResolvedValue(refreshedHead);
    const { result } = renderHook(() =>
      useRemoteContributionResolution("session-1", mocks.refreshProviderEvidence),
    );

    act(() => result.current.requestReplace(TARGET));
    await act(async () => {
      await result.current.confirm();
    });

    expect(mocks.refreshProviderEvidence).toHaveBeenCalledOnce();
    expect(result.current.pending?.expectedRemoteHead).toBe(refreshedHead);
    expect(result.current.error).toBe("lease_mismatch");
  });

  it("clears a pending confirmation without calling Git", () => {
    const { result } = renderHook(() => useRemoteContributionResolution("session-1"));

    act(() => result.current.requestUse(TARGET));
    act(() => result.current.cancel());

    expect(result.current.pending).toBeNull();
    expect(mocks.useRemoteContribution).not.toHaveBeenCalled();
  });
});

describe("classifyRemoteContributionError", () => {
  it.each(["lease_mismatch"])("maps %s to a lease mismatch", (errorCode) => {
    expect(classifyRemoteContributionError(errorCode)).toBe("lease_mismatch");
  });

  it("maps dirty-worktree error codes to the clean-tree instruction", () => {
    expect(classifyRemoteContributionError("dirty_worktree")).toBe("dirty_worktree");
  });

  it("does not infer a category from provider error text", () => {
    expect(classifyRemoteContributionError("remote contribution head changed")).toBe("generic");
  });

  it("keeps unrelated provider errors generic", () => {
    expect(classifyRemoteContributionError("remote_unavailable")).toBe("generic");
    expect(classifyRemoteContributionError(undefined)).toBe("generic");
  });
});

describe("buildRemoteContributionResolutionTarget", () => {
  it("uses the selected PR identity when the repository scope is empty", () => {
    expect(
      buildRemoteContributionResolutionTarget(
        relationWithProviderHead,
        "",
        { owner: "acme", repo: "widget" } as TaskPR,
        "Remote repository",
      ),
    ).toEqual({
      expectedRemoteHead: TARGET.expectedRemoteHead,
      repo: "",
      repositoryName: "acme/widget",
    });
  });

  it("does not create a target without a resolvable contribution action", () => {
    expect(
      buildRemoteContributionResolutionTarget(
        { ...relationWithProviderHead, canReplaceRemote: false, canUseRemote: false },
        "frontend",
        null,
        "Remote repository",
      ),
    ).toBeNull();
  });
});
