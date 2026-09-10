import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useDiscoveredRepositories } from "./use-discovered-repositories";
import type { LocalRepository } from "@/lib/types/http";
import {
  getRepositoryDiscoveryAction,
  refreshRepositoryDiscoveryAction,
} from "@/app/actions/workspaces";
import { repositoryDiscoveryCoordinator } from "@/hooks/domains/workspace/use-repository-discovery";

vi.mock("@/app/actions/workspaces", () => ({
  getRepositoryDiscoveryAction: vi.fn(),
  refreshRepositoryDiscoveryAction: vi.fn(),
}));

const mockDiscover = vi.mocked(getRepositoryDiscoveryAction);

const repoA: LocalRepository = { path: "/work/a", name: "a", default_branch: "main" };
const repoB: LocalRepository = { path: "/work/b", name: "b", default_branch: "main" };

function deferred() {
  let resolve!: (v: { roots: never[]; repositories: LocalRepository[]; total: number }) => void;
  const promise = new Promise<{ roots: never[]; repositories: LocalRepository[]; total: number }>(
    (r) => {
      resolve = r;
    },
  );
  return { promise, resolve };
}

function renderDiscovery(open: boolean, ws: string | null) {
  return renderHook(
    ({ o, w }: { o: boolean; w: string | null }) => useDiscoveredRepositories(o, w),
    {
      initialProps: { o: open, w: ws },
    },
  );
}

describe("useDiscoveredRepositories", () => {
  beforeEach(() => {
    repositoryDiscoveryCoordinator.dispose();
    mockDiscover.mockReset();
    vi.mocked(refreshRepositoryDiscoveryAction).mockReset();
  });

  afterEach(() => {
    repositoryDiscoveryCoordinator.dispose();
  });

  it("returns null until discovery resolves, then the repos", async () => {
    mockDiscover.mockResolvedValue({ roots: [], repositories: [repoA], total: 1 } as never);
    const { result } = renderDiscovery(true, "ws1");
    expect(result.current).toBeNull();
    await waitFor(() => expect(result.current).toEqual([repoA]));
    expect(mockDiscover).toHaveBeenCalledTimes(1);
  });

  it("does not fetch while closed", () => {
    renderDiscovery(false, "ws1");
    expect(mockDiscover).not.toHaveBeenCalled();
  });

  it("keeps the cached result after closing and reopening the popover", async () => {
    const first = deferred();
    mockDiscover.mockReturnValueOnce(first.promise as never);
    const { result, rerender } = renderDiscovery(true, "ws1");

    // Close before the first request resolves. The coordinator keeps the
    // response because it is a cache, not a component-local request.
    rerender({ o: false, w: "ws1" });
    await act(async () => first.resolve({ roots: [], repositories: [repoA], total: 1 }));
    expect(result.current).toBeNull();

    // Reopen: the cached result is immediately available and no second scan
    // is needed while the cache is fresh.
    rerender({ o: true, w: "ws1" });
    await waitFor(() => expect(result.current).toEqual([repoA]));
    expect(mockDiscover).toHaveBeenCalledTimes(1);
  });

  it("never surfaces another workspace's results after a switch", async () => {
    const slowWs1 = deferred();
    mockDiscover.mockReturnValueOnce(slowWs1.promise as never);
    mockDiscover.mockResolvedValueOnce({ roots: [], repositories: [repoB], total: 1 } as never);
    const { result, rerender } = renderDiscovery(true, "ws1");

    // Switch workspaces while ws1's request is still in flight.
    rerender({ o: true, w: "ws2" });
    expect(result.current).toBeNull();
    await waitFor(() => expect(result.current).toEqual([repoB]));

    // ws1's late response must not clobber ws2's result.
    await act(async () => slowWs1.resolve({ roots: [], repositories: [repoA], total: 1 }));
    expect(result.current).toEqual([repoB]);
  });
});
