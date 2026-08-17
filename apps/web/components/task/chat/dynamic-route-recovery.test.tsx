import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  setTaskSession: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { setTaskSession: typeof mocks.setTaskSession }) => unknown) =>
    selector({ setTaskSession: mocks.setTaskSession }),
}));

import type { TaskSession } from "@/lib/types/http";
import { DynamicRouteRecovery } from "./dynamic-route-recovery";

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

describe("DynamicRouteRecovery", () => {
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
});
