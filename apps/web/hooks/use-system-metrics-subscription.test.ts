import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSystemMetricsSubscription } from "./use-system-metrics-subscription";

const clients = vi.hoisted(() => ({
  active: { subscribeSystemMetrics: vi.fn() },
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => clients.active,
  useWebSocketClient: () => clients.active,
}));

describe("useSystemMetricsSubscription", () => {
  beforeEach(() => {
    clients.active = { subscribeSystemMetrics: vi.fn(() => vi.fn()) };
  });

  it("replaces the subscription when the websocket client changes", () => {
    const firstClient = clients.active;
    const { rerender } = renderHook(() => useSystemMetricsSubscription(true));
    const secondClient = { subscribeSystemMetrics: vi.fn(() => vi.fn()) };

    clients.active = secondClient;
    rerender();

    expect(firstClient.subscribeSystemMetrics.mock.results[0]?.value).toHaveBeenCalledOnce();
    expect(secondClient.subscribeSystemMetrics).toHaveBeenCalledOnce();
  });
});
