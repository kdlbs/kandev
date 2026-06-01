import { describe, it, expect, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerFeaturesBridge } from "./features";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * Bridge tests for the features domain.
 *
 * The features bridge is intentionally a no-op because the backend never
 * pushes feature-flag changes over WebSocket. These tests assert the
 * contract: the registrar runs without error, returns a cleanup function,
 * and does not register any WS handlers.
 */
describe("registerFeaturesBridge", () => {
  it("returns a cleanup function without throwing", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    const ws = {} as WebSocketClient;

    expect(() => {
      const cleanup = registerFeaturesBridge(ws, qc);
      expect(typeof cleanup).toBe("function");
      // Cleanup must also be callable without error.
      expect(() => cleanup()).not.toThrow();
    }).not.toThrow();
  });

  it("does not mutate the query cache on registration", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    const ws = {} as WebSocketClient;
    const setSpy = vi.spyOn(qc, "setQueryData");

    registerFeaturesBridge(ws, qc);

    expect(setSpy).not.toHaveBeenCalled();
  });
});
