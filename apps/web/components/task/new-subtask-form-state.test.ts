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
});
