import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { BuiltinMCPManagementView } from "@/lib/types/builtin-mcp";

const api = vi.hoisted(() => ({ get: vi.fn(), policy: vi.fn(), preferences: vi.fn() }));
vi.mock("@/lib/api/domains/builtin-mcp-api", () => ({
  getBuiltinMCPManagement: api.get,
  saveBuiltinMCPPolicy: api.policy,
  saveBuiltinMCPPreferences: api.preferences,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({ auth: { mode: "local", authenticated: true, user: { id: "human" } } }),
}));
import { useBuiltinMCPManagement } from "./use-builtin-mcp-management";

function snapshot(workspace = "ws", task = "task"): BuiltinMCPManagementView {
  return {
    policy: {
      workspace_id: workspace,
      version: 1,
      revision: 4,
      packages: {
        configuration: "available",
        "rich-output": "default",
        walkthroughs: "default",
        diagnostics: "default",
      },
    },
    preferences: { workspace_id: workspace, task_id: task, version: 1, revision: 2, packages: {} },
    catalog: [],
    can_edit: true,
    surface: "kanban-task",
  };
}
beforeEach(() => {
  vi.resetAllMocks();
  api.get.mockResolvedValue(snapshot());
});

describe("built-in MCP management drafts", () => {
  it("saves explicit task consent with both captured revisions", async () => {
    const saved = snapshot();
    saved.preferences!.revision = 3;
    saved.preferences!.packages.configuration = "on";
    api.preferences.mockResolvedValue(saved);
    const { result } = renderHook(() => useBuiltinMCPManagement("tasks", "task", "ws"));
    await waitFor(() => expect(result.current.view).not.toBeNull());
    act(() => result.current.setChoice("configuration", "on"));
    await act(async () => result.current.save());
    expect(api.preferences).toHaveBeenCalledWith("task", 2, 4, { configuration: "on" });
    expect(result.current.view?.preferences?.revision).toBe(3);
  });
  it.each([403, 409])("preserves a denied or conflicting draft (%s)", async (status) => {
    const error = new ApiError("denied", status, {
      error_code: status === 409 ? "builtin_mcp_conflict" : "builtin_mcp_forbidden",
      current: snapshot(),
    });
    api.policy.mockRejectedValue(error);
    const { result } = renderHook(() => useBuiltinMCPManagement("workspaces", "ws", "ws"));
    await waitFor(() => expect(result.current.view).not.toBeNull());
    act(() => result.current.setChoice("walkthroughs", "blocked"));
    await act(async () => {
      await expect(result.current.save()).rejects.toBe(error);
    });
    expect(result.current.draft.walkthroughs).toBe("blocked");
    expect(result.current.error).toBe(error);
    expect(result.current.view?.policy.revision).toBe(4);
  });
  it("rejects a late read from a previous workspace", async () => {
    let resolve!: (view: BuiltinMCPManagementView) => void;
    api.get.mockImplementation((_scope, id) =>
      id === "old"
        ? new Promise((done) => {
            resolve = done;
          })
        : Promise.resolve(snapshot("new")),
    );
    const { result, rerender } = renderHook(
      ({ id }) => useBuiltinMCPManagement("workspaces", id, id),
      { initialProps: { id: "old" } },
    );
    rerender({ id: "new" });
    await waitFor(() => expect(result.current.view?.policy.workspace_id).toBe("new"));
    await act(async () => resolve(snapshot("old")));
    expect(result.current.view?.policy.workspace_id).toBe("new");
  });
  it("rejects a response for another task in the same workspace", async () => {
    api.get.mockResolvedValue(snapshot("ws", "other"));
    const { result } = renderHook(() => useBuiltinMCPManagement("tasks", "task", "ws"));
    await act(async () => {});
    expect(result.current.view).toBeNull();
    expect(result.current.error).not.toBeNull();
  });
});

describe("built-in draft recovery", () => {
  it("discards local edits and resets task overrides without granting Configuration", async () => {
    const initial = snapshot();
    initial.preferences!.packages = { configuration: "on", walkthroughs: "off" };
    api.get.mockResolvedValue(initial);
    const { result } = renderHook(() => useBuiltinMCPManagement("tasks", "task", "ws"));
    await waitFor(() => expect(result.current.view).not.toBeNull());
    act(() => result.current.reset());
    expect(result.current.draft).toEqual({});
    expect(result.current.dirty).toBe(true);
    act(() => result.current.discard());
    expect(result.current.draft).toEqual(initial.preferences!.packages);
    expect(result.current.dirty).toBe(false);
  });
  it("does not save a read-only draft or change required Core", async () => {
    const initial = snapshot();
    initial.can_edit = false;
    api.get.mockResolvedValue(initial);
    const { result } = renderHook(() => useBuiltinMCPManagement("tasks", "task", "ws"));
    await waitFor(() => expect(result.current.view).not.toBeNull());
    act(() => {
      result.current.setChoice("configuration", "on");
      result.current.setChoice("core", "off");
      result.current.reset();
    });
    await act(async () => result.current.save());
    expect(result.current.draft).toEqual({});
    expect(api.preferences).not.toHaveBeenCalled();
  });
  it("ignores a save response after changing tasks", async () => {
    let resolve!: (value: BuiltinMCPManagementView) => void;
    api.preferences.mockReturnValue(
      new Promise((done) => {
        resolve = done;
      }),
    );
    api.get.mockImplementation((_scope, id) => Promise.resolve(snapshot("ws", id)));
    const { result, rerender } = renderHook(
      ({ id }) => useBuiltinMCPManagement("tasks", id, "ws"),
      { initialProps: { id: "old" } },
    );
    await waitFor(() => expect(result.current.view).not.toBeNull());
    act(() => result.current.setChoice("configuration", "on"));
    let pending!: Promise<void>;
    await act(async () => {
      pending = result.current.save();
    });
    rerender({ id: "new" });
    await waitFor(() => expect(result.current.view?.preferences?.task_id).toBe("new"));
    await act(async () => {
      resolve(snapshot("ws", "old"));
      await pending;
    });
    expect(result.current.view?.preferences?.task_id).toBe("new");
    expect(result.current.draft).toEqual({});
  });
});
