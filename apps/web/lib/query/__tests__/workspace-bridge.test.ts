import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerWorkspaceBridge } from "@/lib/query/bridge/workspace";
import { qk } from "@/lib/query/keys";
import type { ListWorkspacesResponse } from "@/lib/types/http";
import { workspaceId as toWsId } from "@/lib/types/ids";

/**
 * Minimal fake WebSocketClient — only the `on` / `off` methods used by the bridge.
 */
type Handler<T> = (msg: T) => void;

function makeFakeWs() {
  const listeners = new Map<string, Set<Handler<unknown>>>();

  return {
    on: vi.fn(<T>(type: string, handler: Handler<T>) => {
      const set = listeners.get(type) ?? new Set();
      set.add(handler as Handler<unknown>);
      listeners.set(type, set);
      return () => {
        const s = listeners.get(type);
        s?.delete(handler as Handler<unknown>);
      };
    }),
    emit: (type: string, message: unknown) => {
      listeners.get(type)?.forEach((h) => h(message));
    },
  };
}

const WS_1 = {
  id: toWsId("ws-1"),
  name: "Workspace One",
  description: null,
  owner_id: "user-1",
  default_executor_id: null,
  default_environment_id: null,
  default_agent_profile_id: null,
  default_config_agent_profile_id: null,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const INITIAL_CACHE: ListWorkspacesResponse = {
  workspaces: [WS_1],
  total: 1,
};

function makeClient(initial?: ListWorkspacesResponse) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  if (initial) {
    qc.setQueryData(qk.workspaces.all(), initial);
  }
  return qc;
}

describe("registerWorkspaceBridge", () => {
  let ws: ReturnType<typeof makeFakeWs>;
  let qc: QueryClient;

  beforeEach(() => {
    ws = makeFakeWs();
    qc = makeClient(INITIAL_CACHE);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- fake ws satisfies the interface
    registerWorkspaceBridge(ws as any, qc);
  });

  it("registers handlers for created/updated/deleted", () => {
    expect(ws.on).toHaveBeenCalledWith("workspace.created", expect.any(Function));
    expect(ws.on).toHaveBeenCalledWith("workspace.updated", expect.any(Function));
    expect(ws.on).toHaveBeenCalledWith("workspace.deleted", expect.any(Function));
  });

  it("workspace.created upserts a new workspace into the cache", () => {
    ws.emit("workspace.created", {
      payload: {
        id: "ws-2",
        name: "Workspace Two",
        description: "desc",
        owner_id: "user-1",
        created_at: "2026-02-01T00:00:00Z",
        updated_at: "2026-02-01T00:00:00Z",
      },
    });

    const data = qc.getQueryData<ListWorkspacesResponse>(qk.workspaces.all());
    expect(data?.workspaces).toHaveLength(2);
    expect(data?.workspaces[0].name).toBe("Workspace Two"); // prepended
  });

  it("workspace.created updates existing workspace (idempotent)", () => {
    ws.emit("workspace.created", {
      payload: {
        id: "ws-1",
        name: "Updated Name",
        owner_id: "user-1",
      },
    });

    const data = qc.getQueryData<ListWorkspacesResponse>(qk.workspaces.all());
    expect(data?.workspaces).toHaveLength(1);
    expect(data?.workspaces[0].name).toBe("Updated Name");
  });

  it("workspace.updated patches only the matching workspace", () => {
    ws.emit("workspace.updated", {
      payload: {
        id: "ws-1",
        name: "Patched Name",
        description: "new desc",
      },
    });

    const data = qc.getQueryData<ListWorkspacesResponse>(qk.workspaces.all());
    expect(data?.workspaces[0].name).toBe("Patched Name");
    expect(data?.workspaces[0].description).toBe("new desc");
  });

  it("workspace.updated is a no-op when cache has no data", () => {
    // Create a fresh client with no pre-seeded cache.
    const freshQc = makeClient();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    registerWorkspaceBridge(ws as any, freshQc);
    ws.emit("workspace.updated", { payload: { id: "ws-1", name: "X" } });
    const data = freshQc.getQueryData<ListWorkspacesResponse>(qk.workspaces.all());
    expect(data).toBeUndefined();
  });

  it("workspace.deleted removes the workspace from the list", () => {
    ws.emit("workspace.deleted", { payload: { id: "ws-1" } });

    const data = qc.getQueryData<ListWorkspacesResponse>(qk.workspaces.all());
    expect(data?.workspaces).toHaveLength(0);
    expect(data?.total).toBe(0);
  });

  it("cleanup function removes all handlers", () => {
    qc = makeClient(INITIAL_CACHE);
    const ws2 = makeFakeWs();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const cleanup = registerWorkspaceBridge(ws2 as any, qc);
    cleanup();

    // After cleanup, events must NOT update the cache
    ws2.emit("workspace.deleted", { payload: { id: "ws-1" } });
    const data = qc.getQueryData<ListWorkspacesResponse>(qk.workspaces.all());
    expect(data?.workspaces).toHaveLength(1); // unchanged
  });
});
