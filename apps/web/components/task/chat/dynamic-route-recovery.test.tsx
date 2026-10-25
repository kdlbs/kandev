import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  setTaskSession: vi.fn(),
  liveSession: undefined as TaskSession | undefined,
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({
      taskSessions: { items: { "session-1": mocks.liveSession } },
      setTaskSession: mocks.setTaskSession,
    }),
  }),
  useAppStore: (selector: (state: { setTaskSession: typeof mocks.setTaskSession }) => unknown) =>
    selector({ setTaskSession: mocks.setTaskSession }),
}));

import type { TaskSession } from "@/lib/types/http";
import { WebSocketRequestError } from "@/lib/ws/client";
import { DynamicRouteRecovery } from "./dynamic-route-recovery";

const RETRY_TEST_ID = "dynamic-route-retry";

const session = {
  id: "session-1",
  task_id: "task-1",
  route_generation: 3,
  route_state: "action_required",
  route_reason: "provider_unavailable",
  state: "WAITING_FOR_INPUT",
  started_at: "2026-08-15T12:00:00Z",
  updated_at: "2026-08-15T12:00:00Z",
} as TaskSession;

beforeEach(() => {
  mocks.liveSession = session;
  mocks.request.mockResolvedValue({
    execution_profile_id: "profile-2",
    route_generation: 4,
    state: "starting",
    reason: "candidate_order",
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("DynamicRouteRecovery response ordering", () => {
  it("applies a same-generation response when no later projection arrived", async () => {
    mocks.request.mockResolvedValueOnce({ route_generation: 3, state: "retrying" });
    render(<DynamicRouteRecovery session={session} />);
    fireEvent.click(screen.getByTestId(RETRY_TEST_ID));
    await waitFor(() =>
      expect(mocks.setTaskSession).toHaveBeenCalledWith(
        expect.objectContaining({
          route_generation: 3,
          route_state: "retrying",
          updated_at: session.updated_at,
        }),
      ),
    );
  });

  it("does not restore a session removed during an action", async () => {
    let finish!: (result: unknown) => void;
    mocks.request.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    render(<DynamicRouteRecovery session={session} />);
    fireEvent.click(screen.getByTestId(RETRY_TEST_ID));
    mocks.liveSession = undefined;
    await act(async () => {
      finish({ route_generation: 4, state: "starting" });
    });
    expect(mocks.setTaskSession).not.toHaveBeenCalled();
  });

  it("keeps a later same-generation failure when the retry response finishes last", async () => {
    let finish!: (result: unknown) => void;
    mocks.request.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    render(<DynamicRouteRecovery session={session} />);
    fireEvent.click(screen.getByTestId(RETRY_TEST_ID));
    mocks.liveSession = {
      ...session,
      route_state: "action_required",
      route_reason: "unclassified_manual_recovery",
      error_message: "The retried turn failed",
      updated_at: "2026-08-15T12:00:02Z",
    };
    await act(async () => {
      finish({ route_generation: 3, state: "active", reason: "retry_due" });
    });
    expect(mocks.setTaskSession).not.toHaveBeenCalled();
    expect((screen.getByTestId(RETRY_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
  });

  it("applies a newer generation without overwriting intervening session fields", async () => {
    let finish!: (result: unknown) => void;
    mocks.request.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    render(<DynamicRouteRecovery session={session} />);
    fireEvent.click(screen.getByTestId("dynamic-route-try-next"));
    mocks.liveSession = { ...session, error_message: "Current diagnostic" };
    await act(async () => {
      finish({ route_generation: 4, state: "starting", execution_profile_id: "profile-2" });
    });
    expect(mocks.setTaskSession).toHaveBeenCalledWith(
      expect.objectContaining({
        route_generation: 4,
        route_state: "starting",
        error_message: "Current diagnostic",
      }),
    );
  });
});

describe("DynamicRouteRecovery actions", () => {
  it("uses one atomic route action request for Skip now", async () => {
    render(<DynamicRouteRecovery session={session} />);

    fireEvent.click(screen.getByTestId("dynamic-route-try-next"));

    await waitFor(() => {
      expect(mocks.request).toHaveBeenCalledTimes(1);
    });
    expect(mocks.request).toHaveBeenCalledWith(
      "session.route_action",
      {
        session_id: "session-1",
        action: "skip",
        expected_generation: 3,
      },
      30000,
    );
    expect(mocks.setTaskSession).toHaveBeenCalledWith(
      expect.objectContaining({ route_generation: 4, route_state: "starting" }),
    );
  });

  it("applies the authoritative route from a conflict response", async () => {
    mocks.request.mockRejectedValueOnce(
      new WebSocketRequestError("route changed", "CONFLICT", {
        route: {
          execution_profile_id: "profile-3",
          route_generation: 5,
          state: "action_required",
          reason: "route_action_launch_failed",
        },
      }),
    );
    render(<DynamicRouteRecovery session={session} />);

    fireEvent.click(screen.getByTestId(RETRY_TEST_ID));

    await waitFor(() => {
      expect(mocks.setTaskSession).toHaveBeenCalledWith(
        expect.objectContaining({
          execution_profile_id: "profile-3",
          route_generation: 5,
          route_state: "action_required",
          route_reason: "route_action_launch_failed",
        }),
      );
    });
  });
});
