import { describe, it, expect, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerCommentsBridge } from "@/lib/query/bridge/comments";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * Bridge test for the comments domain.
 *
 * Comments are client-side-only annotations (no server fetch, no WS events).
 * This test verifies the bridge is a safe no-op: it registers without error,
 * returns a cleanup function, and leaves the QueryClient cache untouched.
 */

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
}

function makeFakeWs(): WebSocketClient {
  return {
    on: vi.fn(),
    off: vi.fn(),
    send: vi.fn(),
    request: vi.fn(),
  } as unknown as WebSocketClient;
}

describe("registerCommentsBridge", () => {
  it("registers without throwing", () => {
    const qc = makeQueryClient();
    const ws = makeFakeWs();
    expect(() => registerCommentsBridge(ws, qc)).not.toThrow();
  });

  it("returns a cleanup function", () => {
    const qc = makeQueryClient();
    const ws = makeFakeWs();
    const cleanup = registerCommentsBridge(ws, qc);
    expect(typeof cleanup).toBe("function");
  });

  it("cleanup does not throw", () => {
    const qc = makeQueryClient();
    const ws = makeFakeWs();
    const cleanup = registerCommentsBridge(ws, qc);
    expect(() => cleanup()).not.toThrow();
  });

  it("does not subscribe to any WS events — comments are client-side only", () => {
    const qc = makeQueryClient();
    const ws = makeFakeWs();
    registerCommentsBridge(ws, qc);
    // WS .on() must NOT have been called — there are no server events for comments
    expect((ws as unknown as { on: ReturnType<typeof vi.fn> }).on).not.toHaveBeenCalled();
  });

  it("does not write anything to the QueryClient cache", () => {
    const qc = makeQueryClient();
    const ws = makeFakeWs();
    registerCommentsBridge(ws, qc);
    // Cache stays empty after registration
    expect(qc.getQueryCache().getAll()).toHaveLength(0);
  });
});
