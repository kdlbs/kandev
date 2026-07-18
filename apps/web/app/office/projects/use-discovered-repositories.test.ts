import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { makeQueryClient } from "@/lib/query/client";
import { useDiscoveredRepositories } from "./use-discovered-repositories";
import { discoverRepositoriesAction } from "@/app/actions/workspaces";
import type { LocalRepository } from "@/lib/types/http";

vi.mock("@/app/actions/workspaces", () => ({
  discoverRepositoriesAction: vi.fn(),
}));

const mockDiscover = vi.mocked(discoverRepositoriesAction);

const repoA: LocalRepository = { path: "/work/a", name: "a", default_branch: "main" };
const repoB: LocalRepository = { path: "/work/b", name: "b", default_branch: "main" };

function deferred() {
  let resolve!: (v: { repositories: LocalRepository[] }) => void;
  const promise = new Promise<{ repositories: LocalRepository[] }>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

function renderDiscovery(open: boolean, ws: string | null) {
  const queryClient = makeQueryClient();
  return renderHook(
    ({ o, w }: { o: boolean; w: string | null }) => useDiscoveredRepositories(o, w),
    {
      initialProps: { o: open, w: ws },
      wrapper: ({ children }: { children: ReactNode }) =>
        createElement(QueryClientProvider, { client: queryClient }, children),
    },
  );
}

describe("useDiscoveredRepositories", () => {
  beforeEach(() => {
    mockDiscover.mockReset();
  });

  it("returns null until discovery resolves, then the repos", async () => {
    mockDiscover.mockResolvedValue({ repositories: [repoA] } as never);
    const { result } = renderDiscovery(true, "ws1");
    expect(result.current).toBeNull();
    await waitFor(() => expect(result.current).toEqual([repoA]));
    expect(mockDiscover).toHaveBeenCalledTimes(1);
  });

  it("does not fetch while closed", () => {
    renderDiscovery(false, "ws1");
    expect(mockDiscover).not.toHaveBeenCalled();
  });

  it("retries after a request interrupted by closing the popover", async () => {
    const first = deferred();
    mockDiscover.mockReturnValueOnce(first.promise as never);
    const { result, rerender } = renderDiscovery(true, "ws1");

    // Close before the first request resolves — its response is discarded.
    rerender({ o: false, w: "ws1" });
    await act(async () => first.resolve({ repositories: [repoA] }));
    expect(result.current).toBeNull();

    // Reopen: must refetch instead of latching "searching" forever.
    mockDiscover.mockResolvedValueOnce({ repositories: [repoB] } as never);
    rerender({ o: true, w: "ws1" });
    await waitFor(() => expect(result.current).toEqual([repoB]));
    expect(mockDiscover).toHaveBeenCalledTimes(2);
  });

  it("never surfaces another workspace's results after a switch", async () => {
    const slowWs1 = deferred();
    mockDiscover.mockReturnValueOnce(slowWs1.promise as never);
    mockDiscover.mockResolvedValueOnce({ repositories: [repoB] } as never);
    const { result, rerender } = renderDiscovery(true, "ws1");

    // Switch workspaces while ws1's request is still in flight.
    rerender({ o: true, w: "ws2" });
    expect(result.current).toBeNull();
    await waitFor(() => expect(result.current).toEqual([repoB]));

    // ws1's late response must not clobber ws2's result.
    await act(async () => slowWs1.resolve({ repositories: [repoA] }));
    expect(result.current).toEqual([repoB]);
  });
});
