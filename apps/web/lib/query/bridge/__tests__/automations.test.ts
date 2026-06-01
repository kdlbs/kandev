import { describe, it, expect, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerAutomationsBridge } from "../automations";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * Bridge tests for the automations domain.
 *
 * The automations domain has no backend push events — the bridge is
 * intentionally a no-op. These tests assert the contract:
 *  1. registerAutomationsBridge returns a cleanup function.
 *  2. The cleanup function does not throw.
 *  3. The bridge does not mutate any query cache entries on registration.
 */
describe("registerAutomationsBridge", () => {
  it("returns a cleanup function", () => {
    const qc = new QueryClient();
    const fakeWs = {} as WebSocketClient;

    const cleanup = registerAutomationsBridge(fakeWs, qc);

    expect(typeof cleanup).toBe("function");
  });

  it("cleanup function does not throw", () => {
    const qc = new QueryClient();
    const fakeWs = {} as WebSocketClient;

    const cleanup = registerAutomationsBridge(fakeWs, qc);

    expect(() => cleanup()).not.toThrow();
  });

  it("does not set any query data on registration", () => {
    const qc = new QueryClient();
    const fakeWs = {} as WebSocketClient;
    const setQueryDataSpy = vi.spyOn(qc, "setQueryData");

    registerAutomationsBridge(fakeWs, qc);

    expect(setQueryDataSpy).not.toHaveBeenCalled();
  });

  it("does not call any methods on the ws client on registration", () => {
    const qc = new QueryClient();
    const fakeWs = {
      on: vi.fn(),
      off: vi.fn(),
      subscribe: vi.fn(),
    } as unknown as WebSocketClient;

    registerAutomationsBridge(fakeWs, qc);

    expect(fakeWs.on).not.toHaveBeenCalled();
    expect(fakeWs.off).not.toHaveBeenCalled();
    expect(fakeWs.subscribe).not.toHaveBeenCalled();
  });
});
