import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

const listCoordinatorsMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-api", () => ({
  listCoordinators: (...args: unknown[]) => listCoordinatorsMock(...args),
}));

const clients = vi.hoisted(() => ({ active: undefined as unknown }));

vi.mock("@/lib/ws/connection", () => ({
  useWebSocketClient: () => clients.active,
}));

import { useCoordinatorSidebarEntries } from "./use-coordinator-sidebar-entries";

const WORKSPACE_ID = "workspace-1";

function coordinator(id: string, openProposals?: number): Coordinator {
  return {
    id,
    workspace_id: WORKSPACE_ID,
    name: `Coordinator ${id}`,
    agent_profile_id: "agent-1",
    executor_profile_id: "exec-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    open_proposals: openProposals,
  };
}

type UpdatedHandler = (message: { payload: Record<string, unknown> }) => void;

function makeWsClient() {
  return {
    on: vi.fn((_type: string, _handler: UpdatedHandler) => vi.fn()),
  };
}

beforeEach(() => {
  listCoordinatorsMock.mockReset();
  clients.active = undefined;
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useCoordinatorSidebarEntries", () => {
  it("seeds badges from each coordinator's open_proposals", async () => {
    listCoordinatorsMock.mockResolvedValue({
      coordinators: [coordinator("c-1", 3), coordinator("c-2", 0), coordinator("c-3")],
    });

    const { result } = renderHook(() => useCoordinatorSidebarEntries(WORKSPACE_ID));

    await waitFor(() => expect(result.current.coordinators).toBeDefined());

    expect(result.current.badgeByCoordinatorId.get("c-1")).toBe(3);
    expect(result.current.badgeByCoordinatorId.get("c-2")).toBe(0);
    expect(result.current.badgeByCoordinatorId.get("c-3")).toBe(0);
  });

  it("does not fetch and returns an empty badge map when workspaceId is null", () => {
    const { result } = renderHook(() => useCoordinatorSidebarEntries(null));

    expect(listCoordinatorsMock).not.toHaveBeenCalled();
    expect(result.current.coordinators).toBeUndefined();
    expect(result.current.badgeByCoordinatorId.size).toBe(0);
  });

  it("replaces a coordinator's badge from a matching coordinator.updated payload", async () => {
    const wsClient = makeWsClient();
    clients.active = wsClient;
    listCoordinatorsMock.mockResolvedValue({ coordinators: [coordinator("c-1", 1)] });

    const { result } = renderHook(() => useCoordinatorSidebarEntries(WORKSPACE_ID));

    await waitFor(() => expect(result.current.badgeByCoordinatorId.get("c-1")).toBe(1));

    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    act(() => {
      handler({
        payload: { workspace_id: WORKSPACE_ID, coordinator_id: "c-1", open_proposals: 5 },
      });
    });

    expect(result.current.badgeByCoordinatorId.get("c-1")).toBe(5);
  });

  it("ignores a coordinator.updated event for a different workspace", async () => {
    const wsClient = makeWsClient();
    clients.active = wsClient;
    listCoordinatorsMock.mockResolvedValue({ coordinators: [coordinator("c-1", 1)] });

    const { result } = renderHook(() => useCoordinatorSidebarEntries(WORKSPACE_ID));

    await waitFor(() => expect(result.current.badgeByCoordinatorId.get("c-1")).toBe(1));

    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    act(() => {
      handler({
        payload: { workspace_id: "other-workspace", coordinator_id: "c-1", open_proposals: 9 },
      });
    });

    expect(result.current.badgeByCoordinatorId.get("c-1")).toBe(1);
  });

  it("resets overrides when the workspace changes", async () => {
    const wsClient = makeWsClient();
    clients.active = wsClient;
    listCoordinatorsMock.mockResolvedValue({ coordinators: [coordinator("c-1", 1)] });

    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) =>
        useCoordinatorSidebarEntries(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_ID } },
    );

    await waitFor(() => expect(result.current.badgeByCoordinatorId.get("c-1")).toBe(1));

    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    act(() => {
      handler({
        payload: { workspace_id: WORKSPACE_ID, coordinator_id: "c-1", open_proposals: 5 },
      });
    });
    expect(result.current.badgeByCoordinatorId.get("c-1")).toBe(5);

    listCoordinatorsMock.mockResolvedValue({ coordinators: [coordinator("c-1", 2)] });
    rerender({ workspaceId: "workspace-2" });

    await waitFor(() => expect(result.current.badgeByCoordinatorId.get("c-1")).toBe(2));
  });
});
