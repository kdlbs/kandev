import { describe, it, expect, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerJiraBridge } from "./jira";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * Tests for the Jira WS → TQ bridge.
 *
 * Jira is a REST-only integration — there are no WebSocket events for issue
 * watches or config changes. The bridge is intentionally a no-op registrar.
 *
 * These tests verify:
 *   1. The registrar returns a cleanup function (contract).
 *   2. The cleanup function is callable and does not throw.
 *   3. The bridge does NOT subscribe to any WS events.
 *   4. The bridge does NOT call any QueryClient methods on registration.
 *
 * If the backend ever adds `jira.*` WS events, extend these tests with
 * cache-assertion tests and add the handler to registerJiraBridge.
 */
describe("registerJiraBridge", () => {
  it("returns a cleanup function", () => {
    const qc = new QueryClient();
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    const cleanup = registerJiraBridge(fakeWs, qc);

    expect(typeof cleanup).toBe("function");
    qc.clear();
  });

  it("cleanup function does not throw", () => {
    const qc = new QueryClient();
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    const cleanup = registerJiraBridge(fakeWs, qc);

    expect(() => cleanup()).not.toThrow();
    qc.clear();
  });

  it("does not subscribe to any WS events on register", () => {
    const qc = new QueryClient();
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    registerJiraBridge(fakeWs, qc);

    expect(fakeWs.on).not.toHaveBeenCalled();
    qc.clear();
  });

  it("does not call any QueryClient methods on register", () => {
    const qc = new QueryClient();
    const setQueryDataSpy = vi.spyOn(qc, "setQueryData");
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    registerJiraBridge(fakeWs, qc);

    expect(setQueryDataSpy).not.toHaveBeenCalled();
    expect(invalidateSpy).not.toHaveBeenCalled();
    qc.clear();
  });
});
