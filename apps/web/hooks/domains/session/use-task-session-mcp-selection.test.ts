import { ApiError } from "@/lib/api/client";
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { getMCPSelections, listMCPServers, replaceMCPSelections } from "@/lib/api/domains/mcp-api";
import type { MCPSelectionResponse } from "@/lib/types/http-mcp";
import { useTaskSessionMCPSelection } from "./use-task-session-mcp-selection";

const WORKSPACE = "workspace-a";
const fixture = vi.hoisted(() => ({
  kanban: { tasks: [{ id: "task-a", workspaceId: "workspace-a", repositories: [] }] },
  workflows: { items: [] },
  taskSessions: { items: { "session-a": { task_id: "task-a", agent_profile_id: "profile-a" } } },
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof fixture) => unknown) => selector(fixture),
}));
vi.mock("@/lib/api/domains/mcp-api", () => ({
  getMCPSelections: vi.fn(),
  listMCPServers: vi.fn(),
  replaceMCPSelections: vi.fn(),
}));
const context = { sessionId: "session-a", taskId: "task-a", workspaceId: WORKSPACE };
function response(
  scope: MCPSelectionResponse["scope"],
  owner: string,
  workspace: string,
): MCPSelectionResponse {
  return {
    scope,
    owner_id: owner,
    workspace_id: workspace,
    definition_ids: [],
    ...(scope === "task_session"
      ? {
          mcp_state: {
            desired_revision: 2,
            applied_revision: 1,
            apply_state: "pending_idle" as const,
          },
        }
      : {}),
  };
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.resetAllMocks();
  vi.mocked(listMCPServers).mockResolvedValue([]);
  vi.mocked(getMCPSelections).mockImplementation(async (scope, owner, workspace) =>
    response(scope, owner, workspace),
  );
});
afterEach(() => vi.useRealTimers());

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.10
it("keeps pending state observable outside the overlay and polls sequentially", async () => {
  const { result } = renderHook(() => useTaskSessionMCPSelection(context));
  await act(async () => {});
  expect(result.current?.state?.apply_state).toBe("pending_idle");
  let release!: (value: MCPSelectionResponse) => void;
  vi.mocked(getMCPSelections).mockImplementation((scope, owner, workspace) =>
    scope === "task_session"
      ? new Promise((resolve) => {
          release = resolve;
        })
      : Promise.resolve(response(scope, owner, workspace)),
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(500);
  });
  const count = vi.mocked(getMCPSelections).mock.calls.length;
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1500);
  });
  expect(vi.mocked(getMCPSelections).mock.calls.length).toBe(count);
  await act(async () => {
    release({
      ...response("task_session", "session-a", WORKSPACE),
      mcp_state: { desired_revision: 2, applied_revision: 2, apply_state: "applied" },
    });
  });
  expect(result.current?.state?.apply_state).toBe("applied");
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(vi.mocked(getMCPSelections).mock.calls.length).toBe(count);
});

it("retains pending state after refresh failure and recovers on the next poll", async () => {
  const { result } = renderHook(() => useTaskSessionMCPSelection(context));
  await act(async () => {});
  vi.mocked(getMCPSelections).mockRejectedValueOnce(new Error("poll rejected"));
  await act(async () => {
    await vi.advanceTimersByTimeAsync(500);
  });
  expect(result.current?.state?.apply_state).toBe("pending_idle");
  vi.mocked(getMCPSelections).mockImplementation(async (scope, owner, workspace) => ({
    ...response(scope, owner, workspace),
    ...(scope === "task_session"
      ? { mcp_state: { desired_revision: 2, applied_revision: 2, apply_state: "applied" as const } }
      : {}),
  }));
  await act(async () => {
    await vi.advanceTimersByTimeAsync(500);
  });
  expect(result.current?.state?.apply_state).toBe("applied");
});

it("cancels pending polling on unmount", async () => {
  const { result, unmount } = renderHook(() => useTaskSessionMCPSelection(context));
  await act(async () => {});
  expect(result.current?.state?.apply_state).toBe("pending_idle");
  const count = vi.mocked(getMCPSelections).mock.calls.length;
  unmount();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(vi.mocked(getMCPSelections).mock.calls.length).toBe(count);
});

it.each([
  { ...context, taskId: null },
  { ...context, sessionId: null },
  { ...context, workspaceId: "foreign-workspace" },
])(
  "does not fetch selection or catalog without verified task-session context %j",
  async (input) => {
    const { result } = renderHook(() => useTaskSessionMCPSelection(input));
    await act(async () => {});
    expect(result.current).toBeNull();
    expect(getMCPSelections).not.toHaveBeenCalled();
    expect(listMCPServers).not.toHaveBeenCalled();
  },
);

it("stops polling when task context is invalidated, including a held response", async () => {
  const { result, rerender } = renderHook((input) => useTaskSessionMCPSelection(input), {
    initialProps: context,
  });
  await act(async () => {});
  let release!: (value: MCPSelectionResponse) => void;
  vi.mocked(getMCPSelections).mockImplementation((scope, owner, workspace) =>
    scope === "task_session"
      ? new Promise((resolve) => {
          release = resolve;
        })
      : Promise.resolve(response(scope, owner, workspace)),
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(500);
  });
  const count = vi.mocked(getMCPSelections).mock.calls.length;
  rerender({ ...context, workspaceId: "foreign-workspace" });
  expect(result.current).toBeNull();
  await act(async () => {
    release(response("task_session", "session-a", WORKSPACE));
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(result.current).toBeNull();
  expect(vi.mocked(getMCPSelections).mock.calls.length).toBe(count);
});

it("stops pending refreshes when shared API recovery owns the error", async () => {
  const { result } = renderHook(() => useTaskSessionMCPSelection(context));
  await act(async () => {});
  const error = new ApiError("stale", 403, { error_code: "interim_settings_interlock_required" });
  error.handled = true;
  vi.mocked(getMCPSelections).mockRejectedValueOnce(error);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(500);
  });
  expect(result.current?.error).toBe(error);
  expect(result.current?.state?.apply_state).toBe("pending_idle");
  const count = vi.mocked(getMCPSelections).mock.calls.length;
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(vi.mocked(getMCPSelections).mock.calls.length).toBe(count);
});

it.each(["deferred_restart", "failed"] as const)(
  "refreshes %s until a later agent start accepts the saved setup",
  async (apply_state) => {
    vi.mocked(getMCPSelections).mockImplementation(async (scope, owner, workspace) => ({
      ...response(scope, owner, workspace),
      ...(scope === "task_session"
        ? { mcp_state: { desired_revision: 2, applied_revision: 1, apply_state } }
        : {}),
    }));
    const { result } = renderHook(() => useTaskSessionMCPSelection(context));
    await act(async () => {});
    expect(result.current?.state?.apply_state).toBe(apply_state);
    vi.mocked(getMCPSelections).mockImplementation(async (scope, owner, workspace) => ({
      ...response(scope, owner, workspace),
      ...(scope === "task_session"
        ? {
            mcp_state: {
              desired_revision: 2,
              applied_revision: 2,
              apply_state: "applied" as const,
            },
          }
        : {}),
    }));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(result.current?.state?.apply_state).toBe("applied");
    const count = vi.mocked(getMCPSelections).mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000);
    });
    expect(vi.mocked(getMCPSelections).mock.calls.length).toBe(count);
  },
);

it("retains a failed session draft and does not poll away its conflict", async () => {
  const conflict = new ApiError("changed", 409, { code: "mcp_selection_conflict" });
  vi.mocked(replaceMCPSelections).mockRejectedValue(conflict);
  const { result } = renderHook(() => useTaskSessionMCPSelection(context));
  await act(async () => {});
  await act(async () => {
    result.current?.onSelectedIdsChange(["docs"]);
  });
  expect(result.current?.selectedIds).toEqual(["docs"]);
  expect(result.current?.savedSelectedIds).toEqual([]);
  const count = vi.mocked(getMCPSelections).mock.calls.length;
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2500);
  });
  expect(vi.mocked(getMCPSelections).mock.calls.length).toBe(count);
  expect(result.current?.error).toBe(conflict);
});
