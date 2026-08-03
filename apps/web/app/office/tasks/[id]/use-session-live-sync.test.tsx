import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSessionLiveSyncSubscriptions } from "./use-session-live-sync";

const mockWebSocketClient = vi.hoisted(() => ({
  subscribeSession: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockWebSocketClient,
}));

describe("Office session live-sync membership", () => {
  beforeEach(() => {
    mockWebSocketClient.subscribeSession.mockReset();
    mockWebSocketClient.subscribeSession.mockImplementation(() => vi.fn());
  });

  it("does not churn subscriptions when equivalent session objects are recreated", () => {
    const first = ["session-b", "session-a"];
    const { rerender, unmount } = renderHook(
      ({ sessionIds }) =>
        useSessionLiveSyncSubscriptions({
          connectionStatus: "connected",
          taskId: "task-1",
          sessionIds,
        }),
      { initialProps: { sessionIds: first } },
    );

    expect(mockWebSocketClient.subscribeSession).toHaveBeenCalledTimes(2);
    rerender({ sessionIds: ["session-a", "session-b"] });
    expect(mockWebSocketClient.subscribeSession).toHaveBeenCalledTimes(2);

    unmount();
    expect(mockWebSocketClient.subscribeSession.mock.results[0]?.value).toHaveBeenCalledTimes(1);
    expect(mockWebSocketClient.subscribeSession.mock.results[1]?.value).toHaveBeenCalledTimes(1);
  });

  it("subscribes and unsubscribes only the membership delta", () => {
    const unsubscriptions = new Map<string, ReturnType<typeof vi.fn>>();
    mockWebSocketClient.subscribeSession.mockImplementation((sessionId: string) => {
      const unsubscribe = vi.fn();
      unsubscriptions.set(sessionId, unsubscribe);
      return unsubscribe;
    });

    const { rerender, unmount } = renderHook(
      ({ sessionIds }) =>
        useSessionLiveSyncSubscriptions({
          connectionStatus: "connected",
          taskId: "task-1",
          sessionIds,
        }),
      { initialProps: { sessionIds: ["session-a", "session-b"] } },
    );

    rerender({ sessionIds: ["session-b", "session-c"] });
    expect(mockWebSocketClient.subscribeSession).toHaveBeenCalledTimes(3);
    expect(unsubscriptions.get("session-a")).toHaveBeenCalledTimes(1);
    expect(unsubscriptions.get("session-b")).toHaveBeenCalledTimes(0);
    expect(unsubscriptions.get("session-c")).toBeDefined();

    unmount();
    expect(unsubscriptions.get("session-b")).toHaveBeenCalledTimes(1);
    expect(unsubscriptions.get("session-c")).toHaveBeenCalledTimes(1);
  });

  it("clears all subscriptions when the connection disconnects", () => {
    const unsubscribe = vi.fn();
    mockWebSocketClient.subscribeSession.mockReturnValue(unsubscribe);
    const initialProps: { connectionStatus: "connected" | "reconnecting" } = {
      connectionStatus: "connected",
    };

    const { rerender, unmount } = renderHook(
      ({ connectionStatus }: { connectionStatus: "connected" | "reconnecting" }) =>
        useSessionLiveSyncSubscriptions({
          connectionStatus,
          taskId: "task-1",
          sessionIds: ["session-a", "session-b"],
        }),
      { initialProps },
    );

    expect(mockWebSocketClient.subscribeSession).toHaveBeenCalledTimes(2);
    rerender({ connectionStatus: "reconnecting" });
    expect(unsubscribe).toHaveBeenCalledTimes(2);
    unmount();
    expect(unsubscribe).toHaveBeenCalledTimes(2);
  });
});
