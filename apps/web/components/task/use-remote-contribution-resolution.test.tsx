import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { GitOperationResult } from "@/hooks/use-git-operations";
import {
  classifyRemoteContributionError,
  useRemoteContributionResolution,
} from "./use-remote-contribution-resolution";

const mocks = vi.hoisted(() => ({
  replaceRemoteContribution: vi.fn(),
  useRemoteContribution: vi.fn(),
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

describe("useRemoteContributionResolution", () => {
  beforeEach(() => {
    mocks.replaceRemoteContribution.mockReset();
    mocks.useRemoteContribution.mockReset();
    mocks.replaceRemoteContribution.mockResolvedValue({
      ...successResult,
      operation: "replace_remote_contribution",
    });
    mocks.useRemoteContribution.mockResolvedValue(successResult);
  });

  it("keeps a typed replacement confirmation pending until it is confirmed", async () => {
    const { result } = renderHook(() => useRemoteContributionResolution("session-1"));

    act(() => result.current.requestReplace(TARGET));
    expect(result.current.pending).toEqual({ action: "replace", ...TARGET });

    await act(async () => {
      await result.current.confirm();
    });

    expect(mocks.replaceRemoteContribution).toHaveBeenCalledWith(TARGET.expectedRemoteHead, "");
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

  it("clears a pending confirmation without calling Git", () => {
    const { result } = renderHook(() => useRemoteContributionResolution("session-1"));

    act(() => result.current.requestUse(TARGET));
    act(() => result.current.cancel());

    expect(result.current.pending).toBeNull();
    expect(mocks.useRemoteContribution).not.toHaveBeenCalled();
  });
});

describe("classifyRemoteContributionError", () => {
  it.each(["remote contribution head changed before adoption", "! [rejected] (stale info)"])(
    "maps %s to a lease mismatch",
    (message) => {
      expect(classifyRemoteContributionError(message)).toBe("lease_mismatch");
    },
  );

  it("maps clean-tree failures to the dirty worktree instruction", () => {
    expect(classifyRemoteContributionError("working tree must be clean")).toBe("dirty_worktree");
  });

  it("keeps unrelated provider errors generic", () => {
    expect(classifyRemoteContributionError("permission denied")).toBe("generic");
    expect(classifyRemoteContributionError(undefined)).toBe("generic");
  });
});
