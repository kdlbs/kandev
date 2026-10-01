import { cleanup } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient, SessionGitRefreshResponse } from "@/lib/ws/client";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { deferred, renderSessionRead } from "./session-read-test-helpers";
import { useSessionGitRefresh } from "./use-session-git-refresh";

const refreshSessionData = vi.fn();
let pendingRequests: Array<ReturnType<typeof deferred<SessionGitRefreshResponse>>> = [];

function cachedStatus(): GitStatusEntry {
  return {
    status_state: "ready",
    files_complete: true,
    detail_state: "ready",
    branch: "main",
    remote_branch: null,
    modified: [],
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files: {},
    tracker_id: "tracker-old",
    tracker_epoch: 1,
    snapshot_revision: 1,
    timestamp: "2026-09-30T10:00:00.000Z",
  };
}

beforeEach(() => {
  pendingRequests = [];
  refreshSessionData.mockReset().mockImplementation(() => {
    const request = deferred<SessionGitRefreshResponse>();
    pendingRequests.push(request);
    return request.promise;
  });
  setWebSocketClient({
    getStatus: () => "connected",
    onConnectionStatus: () => () => undefined,
    refreshSessionData,
  } as unknown as WebSocketClient);
});

afterEach(() => {
  cleanup();
  setWebSocketClient(null);
});

describe("useSessionGitRefresh", () => {
  it("refreshes on activation even when complete membership is cached", () => {
    const { unmount } = renderSessionRead(
      (active: boolean) => useSessionGitRefresh("session", active),
      true,
      (store) => store.getState().setGitStatus("environment", cachedStatus()),
    );

    expect(refreshSessionData).toHaveBeenCalledTimes(1);
    expect(refreshSessionData).toHaveBeenCalledWith("session", "fresh", expect.any(AbortSignal));
    expect(pendingRequests).toHaveLength(1);
    unmount();
  });
});
