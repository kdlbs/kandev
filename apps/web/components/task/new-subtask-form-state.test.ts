import { describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useSubtaskFormState } from "./new-subtask-form-state";

// `useBranchesByURL` triggers a real network ensure() when given a URL — stub
// it so the subtask form-state hook can mount in JSDOM without hitting fetch.
vi.mock("@/hooks/domains/github/use-branches-by-url", () => ({
  useBranchesByURL: () => ({
    branches: () => [],
    loading: () => false,
    ensure: () => undefined,
  }),
}));

describe("useSubtaskFormState — remoteRepos seed", () => {
  it("keeps local and remote additions in one ordered selection list", () => {
    const { result } = renderHook(() => useSubtaskFormState("ws-1"));

    act(() => {
      result.current.appendRepositorySelection({
        kind: "local",
        repositoryId: "repo-local",
        branch: "main",
      });
      result.current.appendRepositorySelection({
        kind: "remote",
        url: "https://github.com/acme/remote",
        branch: "develop",
        source: "paste",
      });
      result.current.appendRepositorySelection({
        kind: "local",
        localPath: "/work/second",
        branch: "trunk",
      });
    });

    expect(result.current.repositorySelections.map((selection) => selection.kind)).toEqual([
      "local",
      "remote",
      "local",
    ]);
    expect(
      result.current.repositorySelections.map((selection) =>
        selection.kind === "folder" ? undefined : selection.branch,
      ),
    ).toEqual(["main", "develop", "trunk"]);
  });

  it("seeds one empty remoteRepos row when useRemote toggles on with an empty list", () => {
    const { result } = renderHook(() => useSubtaskFormState("ws-1"));
    expect(result.current.remoteRepos).toHaveLength(0);

    act(() => {
      result.current.setUseRemote(true);
    });

    expect(result.current.remoteRepos).toHaveLength(1);
    expect(result.current.remoteRepos[0]).toMatchObject({ url: "", branch: "", source: "paste" });
  });

  it("preserves remoteRepos rows when toggling Remote → off → on", () => {
    const PASTED_URL = "github.com/owner/repo";
    const { result } = renderHook(() => useSubtaskFormState("ws-1"));

    act(() => {
      result.current.setUseRemote(true);
    });
    const seededKey = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(seededKey!, { url: PASTED_URL });
    });
    expect(result.current.remoteRepos[0]?.url).toBe(PASTED_URL);

    act(() => {
      result.current.setUseRemote(false);
    });
    expect(result.current.remoteRepos[0]?.url).toBe(PASTED_URL);

    act(() => {
      result.current.setUseRemote(true);
    });
    expect(result.current.remoteRepos).toHaveLength(1);
    expect(result.current.remoteRepos[0]?.url).toBe(PASTED_URL);
  });

  it("keeps fresh-branch state writable for local policy branches", () => {
    const { result } = renderHook(() => useSubtaskFormState("ws-1"));

    expect(result.current.freshBranchEnabled).toBe(false);
    act(() => {
      result.current.setFreshBranchEnabled(true);
    });
    expect(result.current.freshBranchEnabled).toBe(true);
  });
});
