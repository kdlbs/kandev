import { describe, it, expect, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerLinearBridge } from "./linear";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * Tests for the linear WS → TQ bridge.
 *
 * The linear domain has no WS events — watches are managed exclusively via
 * REST and mutations invalidate the cache directly. These tests verify:
 *   1. The registrar returns a cleanup function (contract).
 *   2. The cleanup function is a no-op (nothing to unsubscribe).
 *   3. The bridge does NOT subscribe to any WS events.
 *   4. The bridge does NOT call any QueryClient methods on register.
 */
describe("registerLinearBridge", () => {
  it("returns a cleanup function", () => {
    const qc = new QueryClient();
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    const cleanup = registerLinearBridge(fakeWs, qc);

    expect(typeof cleanup).toBe("function");
    qc.clear();
  });

  it("cleanup function does not throw", () => {
    const qc = new QueryClient();
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    const cleanup = registerLinearBridge(fakeWs, qc);

    expect(() => cleanup()).not.toThrow();
    qc.clear();
  });

  it("does not subscribe to any WS events on register", () => {
    const qc = new QueryClient();
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    registerLinearBridge(fakeWs, qc);

    expect(fakeWs.on).not.toHaveBeenCalled();
    qc.clear();
  });

  it("does not call any QueryClient methods on register", () => {
    const qc = new QueryClient();
    const setQueryDataSpy = vi.spyOn(qc, "setQueryData");
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    registerLinearBridge(fakeWs, qc);

    expect(setQueryDataSpy).not.toHaveBeenCalled();
    expect(invalidateSpy).not.toHaveBeenCalled();
    qc.clear();
  });
});
