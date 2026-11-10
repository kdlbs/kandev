import { createElement, Suspense, startTransition, useLayoutEffect, useState } from "react";
import { act, cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listMCPUsage } from "@/lib/api/domains/mcp-api";
import { ApiError } from "@/lib/api/client";
import type { MCPCatalogUsagePage, MCPOwnerUsage } from "@/lib/types/http-mcp";
import { useMCPUsage } from "./use-mcp-usage";
vi.mock("@/lib/api/domains/mcp-api", () => ({ listMCPUsage: vi.fn() }));
const read = vi.mocked(listMCPUsage);
function owner(id: string, workspace = "w"): MCPOwnerUsage {
  return {
    scope: "task",
    owner_id: id,
    workspace_id: workspace,
    display_name: id,
    owner_state: "available",
    destination: null,
  };
}
function page(
  items: MCPOwnerUsage[],
  next_cursor?: string,
  workspace_id = "w",
): MCPCatalogUsagePage {
  return { workspace_id, definition_id: "d", items, next_cursor };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);

describe("MCP usage reads", () => {
  // @covers AC-AGENTS-MCP-MANAGEMENT-001.4
  it("continues past an empty partial page and deduplicates visible owners", async () => {
    read
      .mockResolvedValueOnce(page([], "first"))
      .mockResolvedValueOnce(page([owner("one")], "second"))
      .mockResolvedValueOnce(page([owner("one"), owner("two")]));
    const { result } = renderHook(() => useMCPUsage("w", "d"));
    await waitFor(() => expect(result.current.hasMore).toBe(true));
    expect(result.current.items).toEqual([]);
    await act(() => result.current.loadMore());
    await act(() => result.current.loadMore());
    expect(result.current.items.map((row) => row.owner_id)).toEqual(["one", "two"]);
    expect(result.current.hasMore).toBe(false);
  });
  it("retains stale rows after refresh failure and clears them after access loss", async () => {
    read.mockResolvedValueOnce(page([owner("one")]));
    const { result } = renderHook(() => useMCPUsage("w", "d"));
    await waitFor(() => expect(result.current.items).toHaveLength(1));
    read.mockRejectedValueOnce(new Error("offline"));
    await act(() => result.current.reload());
    expect(result.current.items).toHaveLength(1);
    expect(result.current.error).toBeTruthy();
    read.mockRejectedValueOnce(new ApiError("not found", 404, null));
    await act(() => result.current.reload());
    expect(result.current.items).toEqual([]);
    expect(result.current.unavailable).toBe(true);
  });
  // @covers AC-AGENTS-MCP-MANAGEMENT-001.3
  it("masks the previous workspace immediately and ignores its late completion", async () => {
    const pending = deferred<MCPCatalogUsagePage>();
    read
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValueOnce(page([owner("new", "other")], undefined, "other"));
    const { result, rerender } = renderHook(({ workspace }) => useMCPUsage(workspace, "d"), {
      initialProps: { workspace: "w" },
    });
    rerender({ workspace: "other" });
    expect(result.current.items).toEqual([]);
    await waitFor(() => expect(result.current.items[0]?.owner_id).toBe("new"));
    await act(async () => pending.resolve(page([owner("old")])));
    expect(result.current.items[0]?.owner_id).toBe("new");
    expect(result.current.busy).toBe(false);
  });
  it("serializes repeated load-more and lets a refresh replace the page sequence", async () => {
    const pending = deferred<MCPCatalogUsagePage>();
    read
      .mockResolvedValueOnce(page([owner("one")], "next"))
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValueOnce(page([owner("fresh")]));
    const { result } = renderHook(() => useMCPUsage("w", "d"));
    await waitFor(() => expect(result.current.hasMore).toBe(true));
    let more!: Promise<void>;
    act(() => {
      more = result.current.loadMore();
      void result.current.loadMore();
    });
    expect(read).toHaveBeenCalledTimes(2);
    await act(() => result.current.reload());
    await act(async () => {
      pending.resolve(page([owner("old")]));
      await more;
    });
    expect(result.current.items.map((row) => row.owner_id)).toEqual(["fresh"]);
    expect(result.current.busy).toBe(false);
  });
});

// @covers AC-AGENTS-MCP-MANAGEMENT-001.3, AC-AGENTS-MCP-MANAGEMENT-001.4
it("keeps stale destinations disabled until a refresh retry succeeds", async () => {
  read.mockResolvedValueOnce(page([owner("one")], "next"));
  const { result } = renderHook(() => useMCPUsage("w", "d"));
  await waitFor(() => expect(result.current.items).toHaveLength(1));
  read.mockRejectedValueOnce(new Error("offline"));
  await act(() => result.current.reload());
  await act(() => result.current.loadMore());
  expect(read).toHaveBeenCalledTimes(2);
  const retry = deferred<MCPCatalogUsagePage>();
  read.mockReturnValueOnce(retry.promise);
  let pending!: Promise<void>;
  act(() => {
    pending = result.current.reload();
  });
  expect(result.current.stale).toBe(true);
  await act(async () => {
    retry.resolve(page([owner("fresh")]));
    await pending;
  });
  expect(result.current.stale).toBe(false);
  expect(result.current.items[0].owner_id).toBe("fresh");
});
it("retains accepted pages and their links after a next-page failure", async () => {
  read.mockResolvedValueOnce(page([owner("one")], "next"));
  const { result } = renderHook(() => useMCPUsage("w", "d"));
  await waitFor(() => expect(result.current.hasMore).toBe(true));
  read.mockRejectedValueOnce(new Error("offline"));
  await act(() => result.current.loadMore());
  expect(result.current.error).toBeNull();
  expect(result.current.pageError).toBeTruthy();
  expect(result.current.stale).toBe(false);
  expect(result.current.items[0].owner_id).toBe("one");
});

it("accepts the committed usage response while another context render is suspended", async () => {
  const pending = deferred<MCPCatalogUsagePage>();
  read.mockReturnValueOnce(pending.promise);
  const never = new Promise<void>(() => {});
  let current!: ReturnType<typeof useMCPUsage>;
  let select!: (workspace: string) => void;
  let suspended = false;
  function Consumer({ workspace }: { workspace: string }) {
    const usage = useMCPUsage(workspace, "d");
    useLayoutEffect(() => {
      current = usage;
    });
    if (workspace === "speculative") {
      suspended = true;
      throw never;
    }
    return createElement("span", null, workspace);
  }
  function Harness() {
    const [workspace, setWorkspace] = useState("w");
    select = setWorkspace;
    return createElement(Suspense, { fallback: null }, createElement(Consumer, { workspace }));
  }
  render(createElement(Harness));
  act(() => startTransition(() => select("speculative")));
  expect(suspended).toBe(true);
  await act(async () => pending.resolve(page([owner("committed")])));
  expect(current.items.map((item) => item.owner_id)).toEqual(["committed"]);
  expect(current.busy).toBe(false);
});
