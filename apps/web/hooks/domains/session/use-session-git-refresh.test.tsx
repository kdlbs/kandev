import { act, cleanup } from "@testing-library/react";
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

function unavailableResponse(): SessionGitRefreshResponse {
  return {
    success: false,
    session_id: "session",
    task_environment_id: "environment",
    mode: "fresh",
    status_state: "unavailable",
    snapshots: [],
  };
}

function readyResponse(): SessionGitRefreshResponse {
  return {
    success: true,
    session_id: "session",
    task_environment_id: "environment",
    mode: "fresh",
    status_state: "ready",
    snapshots: [
      {
        type: "notification",
        action: "session.git.event",
        payload: {
          type: "status_update",
          session_id: "session",
          task_environment_id: "environment",
          timestamp: "2026-09-30T10:00:01.000Z",
          status: {
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
            remote_ahead: 0,
            remote_behind: 0,
            tracker_id: "tracker-old",
            tracker_epoch: 1,
            snapshot_revision: 2,
            files: {},
          },
        },
      },
    ],
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
  vi.restoreAllMocks();
});

describe("useSessionGitRefresh", () => {
  it("refreshes on activation even when complete membership is cached", () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
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

  it("waits for focus before reading in a visible but unfocused window", async () => {
    const hasFocus = vi.spyOn(document, "hasFocus").mockReturnValue(false);
    const hook = renderSessionRead<void, boolean>(
      (active: boolean) => useSessionGitRefresh("session", active),
      true,
      (store) => store.getState().setGitStatus("environment", cachedStatus()),
    );

    try {
      expect(document.visibilityState).toBe("visible");
      expect(refreshSessionData).not.toHaveBeenCalled();

      hasFocus.mockReturnValue(true);
      act(() => window.dispatchEvent(new Event("focus")));
      await vi.waitFor(() => expect(refreshSessionData).toHaveBeenCalledOnce());
      expect(refreshSessionData).toHaveBeenCalledWith("session", "fresh", expect.any(AbortSignal));
    } finally {
      hook.unmount();
    }
  });

  it("cancels delayed recovery while blurred and refreshes again on focus", async () => {
    vi.useFakeTimers();
    const hasFocus = vi.spyOn(document, "hasFocus").mockReturnValue(true);
    const hook = renderSessionRead<void, boolean>(
      (active: boolean) => useSessionGitRefresh("session", active),
      true,
      (store) => store.getState().setGitStatus("environment", cachedStatus()),
    );

    try {
      pendingRequests[0].resolve(unavailableResponse());
      await vi.waitFor(() => expect(pendingRequests).toHaveLength(2));
      pendingRequests[1].resolve(unavailableResponse());
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0);
      });

      act(() => {
        window.dispatchEvent(new Event("blur"));
      });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(5_000);
      });
      expect(pendingRequests).toHaveLength(2);

      act(() => {
        window.dispatchEvent(new Event("focus"));
      });
      await vi.waitFor(() => expect(pendingRequests).toHaveLength(3));
      pendingRequests[2].resolve(readyResponse());
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0);
      });
      expect(pendingRequests).toHaveLength(3);
      expect(
        hook.result.current.store.getState().gitStatus.byEnvironmentId.environment?.status_state,
      ).toBe("ready");
    } finally {
      hasFocus.mockRestore();
      hook.unmount();
      vi.useRealTimers();
    }
  });

  it("releases the recovery schedule when the Changes surface becomes inactive", async () => {
    vi.useFakeTimers();
    const hasFocus = vi.spyOn(document, "hasFocus").mockReturnValue(true);
    const hook = renderSessionRead<void, boolean>(
      (active: boolean) => useSessionGitRefresh("session", active),
      true,
    );

    try {
      pendingRequests[0].resolve(unavailableResponse());
      await vi.waitFor(() => expect(pendingRequests).toHaveLength(2));
      pendingRequests[1].resolve(unavailableResponse());
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0);
      });

      hook.rerender(false);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(5_000);
      });
      expect(pendingRequests).toHaveLength(2);
    } finally {
      hasFocus.mockRestore();
      hook.unmount();
      vi.useRealTimers();
    }
  });
});
