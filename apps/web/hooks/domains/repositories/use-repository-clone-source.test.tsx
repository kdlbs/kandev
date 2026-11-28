import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useRepositoryCloneSource } from "./use-repository-clone-source";

const inspectRepositoryCloneSource = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/workspace-api", () => ({ inspectRepositoryCloneSource }));

afterEach(() => {
  inspectRepositoryCloneSource.mockReset();
});

const ready = {
  ready: true,
  origin: "https://github.com/acme/api.git",
  default_branch: "main",
  branches: [{ name: "main", type: "remote", remote: "origin" }],
};

describe("useRepositoryCloneSource", () => {
  it("fences an older inspection when the visible candidate changes", async () => {
    let resolveFirst: ((value: typeof ready) => void) | undefined;
    let resolveSecond: ((value: typeof ready) => void) | undefined;
    inspectRepositoryCloneSource.mockImplementation(
      (_workspaceId: string, payload: { repositoryId?: string }) => {
        return new Promise((resolve) => {
          if (payload.repositoryId === "repo-1") resolveFirst = resolve;
          else resolveSecond = resolve;
        });
      },
    );
    const view = renderHook(
      ({ candidates }: { candidates: Array<{ key: string; repositoryId?: string }> }) =>
        useRepositoryCloneSource("workspace-1", candidates),
      { initialProps: { candidates: [{ key: "row-1", repositoryId: "repo-1" }] } },
    );
    await waitFor(() => expect(inspectRepositoryCloneSource).toHaveBeenCalledTimes(1));

    view.rerender({ candidates: [{ key: "row-2", repositoryId: "repo-2" }] });
    await waitFor(() => expect(inspectRepositoryCloneSource).toHaveBeenCalledTimes(2));
    resolveFirst?.(ready);
    await Promise.resolve();
    expect(view.result.current.states["row-1"]).toBeUndefined();
    expect(view.result.current.states["row-2"]?.status).toBe("checking");

    resolveSecond?.(ready);
    await waitFor(() => expect(view.result.current.states["row-2"]?.status).toBe("ready"));
  });

  it("clears inspection state when the mode is disabled", async () => {
    inspectRepositoryCloneSource.mockResolvedValue(ready);
    const view = renderHook(
      ({
        enabled,
        candidates,
      }: {
        enabled: boolean;
        candidates: [{ key: string; localPath: string }];
      }) => useRepositoryCloneSource("workspace-1", candidates, enabled),
      {
        initialProps: {
          enabled: true,
          candidates: [{ key: "row-1", localPath: "/work/api" }],
        },
      },
    );
    await waitFor(() => expect(view.result.current.states["row-1"]?.status).toBe("ready"));
    view.rerender({
      enabled: false,
      candidates: [{ key: "row-1", localPath: "/work/api" }],
    });
    await waitFor(() => expect(view.result.current.states).toEqual({}));
  });
});
