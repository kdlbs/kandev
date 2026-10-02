import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  listActivity: vi.fn(),
  listMembers: vi.fn(),
  status: { current: "connected" },
  handlers: [] as Array<(m: { payload: Record<string, string> }) => void>,
}));

vi.mock("@/lib/api/domains/coordinator-activity-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/coordinator-activity-api")>()),
  listActivity: mocks.listActivity,
}));
vi.mock("@/lib/api/domains/team-access-api", () => ({ listWorkspaceMembers: mocks.listMembers }));
vi.mock("@/lib/ws/connection", () => {
  const client = {
    on: (_event: string, h: (typeof mocks.handlers)[number]) => {
      mocks.handlers.push(h);
      return () => undefined;
    },
  };
  return { useWebSocketClient: () => client };
});
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: unknown) => unknown) =>
    selector({ connection: { status: mocks.status.current } }),
}));

import { useActivity } from "./use-activity";

const PARAMS = { workspaceId: "ws-1", coordinatorId: "co-1", activityClass: undefined } as const;
const page = { rows: [], next_cursor: null };

beforeEach(() => {
  mocks.listActivity.mockReset();
  mocks.listActivity.mockResolvedValue(page);
  mocks.listMembers.mockReset();
  mocks.listMembers.mockResolvedValue({ members: [] });
  mocks.status.current = "connected";
  mocks.handlers.length = 0;
});

const fire = (payload: Record<string, string>) =>
  act(() => mocks.handlers.forEach((h) => h({ payload })));

describe("useActivity", () => {
  it("loads the first page for the class filter", async () => {
    const { result } = renderHook(() => useActivity({ ...PARAMS, activityClass: "move" }));
    await waitFor(() => expect(result.current.snapshot.status).toBe("loaded"));
    expect(mocks.listActivity).toHaveBeenCalledWith("ws-1", "co-1", { class: "move" });
  });

  it("re-reads for this coordinator's coordinator.updated event only", async () => {
    const { result } = renderHook(() => useActivity(PARAMS));
    await waitFor(() => expect(result.current.snapshot.status).toBe("loaded"));
    expect(mocks.listActivity).toHaveBeenCalledTimes(1);
    fire({ workspace_id: "ws-2", coordinator_id: "co-1" });
    fire({ workspace_id: "ws-1", coordinator_id: "co-2" });
    expect(mocks.listActivity).toHaveBeenCalledTimes(1);
    fire({ workspace_id: "ws-1", coordinator_id: "co-1" });
    await waitFor(() => expect(mocks.listActivity).toHaveBeenCalledTimes(2));
  });

  it("re-reads when the connection returns and not on other status changes", async () => {
    mocks.status.current = "disconnected";
    const { result, rerender } = renderHook(() => useActivity(PARAMS));
    await waitFor(() => expect(result.current.snapshot.status).toBe("loaded"));
    expect(mocks.listActivity).toHaveBeenCalledTimes(1);
    mocks.status.current = "connecting";
    rerender();
    expect(mocks.listActivity).toHaveBeenCalledTimes(1);
    mocks.status.current = "connected";
    rerender();
    await waitFor(() => expect(mocks.listActivity).toHaveBeenCalledTimes(2));
  });

  it("starts a fresh load when the filter changes", async () => {
    const { result, rerender } = renderHook((p) => useActivity(p), {
      initialProps: PARAMS as never,
    });
    await waitFor(() => expect(result.current.snapshot.status).toBe("loaded"));
    rerender({ ...PARAMS, activityClass: "stop" } as never);
    await waitFor(() =>
      expect(mocks.listActivity).toHaveBeenLastCalledWith("ws-1", "co-1", { class: "stop" }),
    );
  });
});
