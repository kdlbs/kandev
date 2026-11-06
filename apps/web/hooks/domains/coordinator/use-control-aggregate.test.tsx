import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { useControlAggregate } from "./use-control-aggregate";

const api = vi.hoisted(() => ({ aggregate: vi.fn() }));
vi.mock("@/lib/api/domains/office-extended-api", () => ({
  getWorkspaceAggregate: api.aggregate,
}));

const wire = {
  workspaces: [
    {
      workspace_id: "alpha",
      name: "Alpha",
      task_count: 2,
      open_tasks: 0,
      in_progress_tasks: 2,
      blocked_tasks: 0,
      done_tasks: 0,
      pending_approvals: 1,
      agent_count: 1,
      running_agents: 0,
    },
  ],
  recent_activity: [
    {
      id: "act-1",
      workspace_id: "alpha",
      actor_type: "agent",
      actor_id: "agent-1",
      action: "task.created",
      target_id: "task-1",
      details: "{}",
      created_at: "2026-10-20T00:00:00Z",
    },
  ],
};

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

beforeEach(() => {
  api.aggregate.mockReset().mockResolvedValue(wire);
});
afterEach(() => cleanup());

describe("useControlAggregate", () => {
  it("does not read the aggregate while disabled", async () => {
    const { result } = renderHook(() => useControlAggregate(false), { wrapper });
    await act(async () => {
      await result.current.refresh();
    });
    expect(api.aggregate).not.toHaveBeenCalled();
    expect(result.current.data).toBeUndefined();
    expect(result.current.error).toBe(false);
  });

  it("reads one aggregate without caching and normalizes the activity feed", async () => {
    const { result } = renderHook(() => useControlAggregate(true), { wrapper });
    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(api.aggregate).toHaveBeenCalledTimes(1);
    expect(api.aggregate.mock.calls[0][0]).toMatchObject({ cache: "no-store" });
    expect(result.current.data?.workspaces).toEqual(wire.workspaces);
    expect(result.current.data?.recentActivity[0]).toMatchObject({
      id: "act-1",
      workspaceId: "alpha",
      actorId: "agent-1",
      targetId: "task-1",
      createdAt: "2026-10-20T00:00:00Z",
    });
  });

  it("keeps the last snapshot and reports the error when a refresh fails", async () => {
    const { result } = renderHook(() => useControlAggregate(true), { wrapper });
    await waitFor(() => expect(result.current.data).toBeDefined());
    api.aggregate.mockRejectedValueOnce(new Error("offline"));
    await act(async () => {
      await result.current.refresh();
    });
    expect(result.current.error).toBe(true);
    expect(result.current.data?.workspaces[0].workspace_id).toBe("alpha");
    await act(async () => {
      await result.current.refresh();
    });
    expect(result.current.error).toBe(false);
  });

  it("treats missing lists in the wire shape as empty", async () => {
    api.aggregate.mockResolvedValue({});
    const { result } = renderHook(() => useControlAggregate(true), { wrapper });
    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.data).toEqual({ workspaces: [], recentActivity: [] });
  });
});
