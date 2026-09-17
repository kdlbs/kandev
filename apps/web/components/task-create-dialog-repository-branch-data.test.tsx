import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Branch } from "@/lib/types/http";
import type { TaskRepoRow } from "./task-create-dialog-types";

const hostBranches: Branch[] = [{ name: "local-only", type: "local" }];

vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useBranches: () => ({
    branches: hostBranches,
    isLoading: false,
    refresh: vi.fn(),
    isLoaded: true,
  }),
}));

vi.mock("./task-create-dialog-repo-branch-autoselect", () => ({
  useRepoBranchAutoselect: vi.fn(),
}));

import { useRepositoryBranchData } from "./task-create-dialog-repository-branch-data";

const row: TaskRepoRow = {
  key: "row-1",
  repositoryId: "repo-1",
  branch: "main",
};

describe("useRepositoryBranchData remote-origin branches", () => {
  it("does not expose host refs before verified remote branches arrive", () => {
    const { result } = renderHook(() =>
      useRepositoryBranchData({
        row,
        workspaceId: "workspace-1",
        onBranchChange: vi.fn(),
        remoteBranches: undefined,
        remoteOriginMode: true,
        remoteOriginInspectionLoading: false,
      }),
    );

    expect(result.current.branches).toEqual([]);
    expect(result.current.branchesLoading).toBe(true);
    expect(result.current.branchesLoaded).toBe(false);
  });

  it("uses the refreshed origin refs and ignores host-only refs in remote mode", () => {
    const remoteBranches: Branch[] = [{ name: "main", type: "remote", remote: "origin" }];
    const { result } = renderHook(() =>
      useRepositoryBranchData({
        row,
        workspaceId: "workspace-1",
        onBranchChange: vi.fn(),
        remoteBranches,
        remoteOriginMode: true,
      }),
    );

    expect(result.current.branches).toEqual(remoteBranches);
    expect(result.current.branches).not.toContainEqual(hostBranches[0]);
    expect(result.current.branchesLoaded).toBe(true);
  });

  it("uses host refs after returning to host execution", () => {
    const staleRemoteBranches: Branch[] = [{ name: "stale", type: "remote", remote: "origin" }];
    const { result } = renderHook(() =>
      useRepositoryBranchData({
        row,
        workspaceId: "workspace-1",
        onBranchChange: vi.fn(),
        remoteBranches: staleRemoteBranches,
        remoteOriginMode: false,
      }),
    );

    expect(result.current.branches).toEqual(hostBranches);
  });
});
