import { describe, it, expect, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerIntegrationsBridge } from "./integrations";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * Tests for the integrations WS → TQ bridge.
 *
 * The integrations domain has no WS events — availability is polled via HTTP
 * (refetchInterval: 90_000) and the enabled toggle is localStorage-backed.
 * These tests verify:
 *   1. The registrar returns a cleanup function (contract).
 *   2. The cleanup function is a no-op (nothing to unsubscribe).
 *   3. The bridge does NOT subscribe to any WS events (no WS side-effects).
 *
 * This file sets the structural pattern for wave 2 workers that WILL have
 * real WS subscriptions. When the backend adds `integration.health.updated`
 * events, this test should be extended with cache-assertion tests.
 */
describe("registerIntegrationsBridge", () => {
  it("returns a cleanup function", () => {
    const qc = new QueryClient();
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    const cleanup = registerIntegrationsBridge(fakeWs, qc);

    expect(typeof cleanup).toBe("function");
    qc.clear();
  });

  it("cleanup function does not throw", () => {
    const qc = new QueryClient();
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    const cleanup = registerIntegrationsBridge(fakeWs, qc);

    expect(() => cleanup()).not.toThrow();
    qc.clear();
  });

  it("does not subscribe to any WS events on register", () => {
    const qc = new QueryClient();
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    registerIntegrationsBridge(fakeWs, qc);

    // The bridge is a no-op: no WS handlers should be registered.
    expect(fakeWs.on).not.toHaveBeenCalled();
    qc.clear();
  });

  it("does not call any QueryClient methods on register", () => {
    const qc = new QueryClient();
    const setQueryDataSpy = vi.spyOn(qc, "setQueryData");
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    const fakeWs = { on: vi.fn(), off: vi.fn() } as unknown as WebSocketClient;

    registerIntegrationsBridge(fakeWs, qc);

    expect(setQueryDataSpy).not.toHaveBeenCalled();
    expect(invalidateSpy).not.toHaveBeenCalled();
    qc.clear();
  });
});
