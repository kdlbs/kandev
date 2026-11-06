import { act, renderHook, cleanup, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { createAppStore } from "@/lib/state/store";
import type { Workspace } from "@/lib/types/http";
import { workspaceId } from "@/lib/types/ids";
import { useWorkspaceClone, canCloneWorkspace } from "./use-workspace-clone";
import { useWorkspaceCollaboration } from "./use-workspace-collaboration";

const action = vi.fn();
const push = vi.fn();
const toast = vi.fn();
let store = createAppStore();
const { readCollaboration, putCollaboration } = vi.hoisted(() => ({
  readCollaboration: vi.fn(),
  putCollaboration: vi.fn(),
}));
vi.mock("@/app/actions/workspaces", () => ({
  cloneWorkspaceAction: (...args: unknown[]) => action(...args),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
  useAppStore: (selector: (state: ReturnType<typeof store.getState>) => unknown) =>
    selector(store.getState()),
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));
vi.mock("@/lib/api/domains/workspace-collaboration-api", () => ({
  COLLABORATION_ACTIONS: ["read", "report", "prompt", "control"],
  readWorkspaceCollaboration: readCollaboration,
  putWorkspaceCollaboration: putCollaboration,
}));
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push }) }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));
const source = {
  id: "source",
  name: "Source",
  owner_id: "owner",
  created_at: "",
  updated_at: "",
  scopes: ["workspace.manage", "secret.manage"],
};
const target: Workspace = {
  ...source,
  id: workspaceId("target"),
  name: "Copy",
  viewer_role: "owner",
  member_count: 1,
  acp_idle_suspension_enabled: false,
  acp_idle_timeout_minutes: 120,
};
const translate = (key: string) => key;

describe("workspace clone flow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    store = createAppStore({ workspaces: { items: [source], activeId: source.id } });
  });
  afterEach(cleanup);
  it("requires management and credential authority and excludes managed workspaces", () => {
    expect(canCloneWorkspace(source)).toBe(true);
    expect(canCloneWorkspace({ ...source, scopes: ["workspace.read"] })).toBe(false);
    expect(canCloneWorkspace({ ...source, office_workflow_id: "office" })).toBe(false);
    expect(canCloneWorkspace({ ...source, name: "Improve Kandev" })).toBe(false);
  });
  it("guards rapid submission and merges into the current live list", async () => {
    let resolve!: (value: Workspace) => void;
    action.mockReturnValue(
      new Promise<Workspace>((done) => {
        resolve = done;
      }),
    );
    const { result } = renderHook(() => useWorkspaceClone(translate));
    act(() => result.current.open(source));
    act(() => result.current.setName(" Copy "));
    let pending!: Promise<void>;
    act(() => {
      pending = result.current.submit();
      void result.current.submit();
    });
    expect(action).toHaveBeenCalledOnce();
    const sibling = { ...source, id: "sibling" };
    act(() => store.getState().setWorkspaces([source, sibling, target]));
    await act(async () => {
      resolve(target);
      await pending;
    });
    expect(store.getState().workspaces.items.map((item) => item.id)).toEqual([
      "target",
      "source",
      "sibling",
    ]);
    expect(store.getState().workspaces.items[0].viewer_role).toBe("owner");
    expect(store.getState().workspaces.activeId).toBe("source");
    expect(push).toHaveBeenCalledWith("/settings/workspaces/target");
  });
  it("retains the draft and explains an uncertain response without retry", async () => {
    action.mockRejectedValue(new TypeError("Network error"));
    const { result } = renderHook(() => useWorkspaceClone(translate));
    act(() => result.current.open(source));
    act(() => result.current.setName("My copy"));
    await act(async () => {
      await result.current.submit();
    });
    expect(result.current.name).toBe("My copy");
    expect(result.current.error).toBe("workspaces:cloneUncertain");
    expect(result.current.source?.id).toBe(source.id);
    expect(action).toHaveBeenCalledOnce();
    expect(push).not.toHaveBeenCalled();
  });
  it("does not submit a blank name and clears cancelled drafts", async () => {
    const { result } = renderHook(() => useWorkspaceClone(translate));
    act(() => result.current.open(source));
    act(() => result.current.setName(" "));
    await act(async () => {
      await result.current.submit();
    });
    expect(action).not.toHaveBeenCalled();
    act(() => result.current.close());
    expect(result.current.source).toBeNull();
    expect(result.current.name).toBe("");
  });
});

const collaborationWorkspace = {
  id: "ws-1",
  name: "Source",
  scopes: ["workspace.read", "workspace.manage"],
};
const collaborationResult = (targetName: string, targetId = "ws-2") => ({
  workspaces: [
    collaborationWorkspace,
    { ...collaborationWorkspace, id: targetId, name: targetName, scopes: ["workspace.read"] },
  ],
  grants: [],
});

describe("workspace collaboration editor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    store = createAppStore();
  });
  afterEach(cleanup);

  it("surfaces a rejected read as denied", async () => {
    readCollaboration.mockRejectedValue(new ApiError("forbidden", 403, {}));
    const { result } = renderHook(() => useWorkspaceCollaboration("task-1", "ws-1", true));
    await waitFor(() => expect(result.current.status).toBe("denied"));
    expect(result.current.rows).toEqual([]);
  });

  it("ignores a read that resolves after the task changes", async () => {
    let resolveFirst!: (value: unknown) => void;
    readCollaboration
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
      )
      .mockResolvedValueOnce(collaborationResult("Second target"));
    const { result, rerender } = renderHook(
      ({ taskId }) => useWorkspaceCollaboration(taskId, "ws-1", true),
      { initialProps: { taskId: "task-a" } },
    );
    await waitFor(() => expect(readCollaboration).toHaveBeenCalledTimes(1));
    rerender({ taskId: "task-b" });
    await waitFor(() => expect(readCollaboration).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.status).toBe("ready"));
    await act(async () => {
      resolveFirst(collaborationResult("Stale target"));
    });
    expect(result.current.rows.map((row) => row.name)).toEqual(["Second target"]);
  });

  it("keeps the draft and marks a failed save", async () => {
    readCollaboration.mockResolvedValue(collaborationResult("Target"));
    putCollaboration.mockRejectedValue(new ApiError("forbidden", 403, {}));
    const { result } = renderHook(() => useWorkspaceCollaboration("task-1", "ws-1", true));
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.edit("ws-2", "read", true));
    await act(async () => {
      await result.current.save("ws-2");
    });
    const row = result.current.rows.find((item) => item.id === "ws-2");
    expect(row?.draft.read).toBe(true);
    expect(row?.saved.read).toBe(false);
    expect(row?.error).toBe("denied");
    expect(row?.saving).toBe(false);
  });

  it("keeps edits made while a save is in flight", async () => {
    readCollaboration.mockResolvedValue(collaborationResult("Target"));
    let resolveSave!: (value: unknown) => void;
    putCollaboration.mockReturnValue(
      new Promise((resolve) => {
        resolveSave = resolve;
      }),
    );
    const { result } = renderHook(() => useWorkspaceCollaboration("task-1", "ws-1", true));
    await waitFor(() => expect(result.current.status).toBe("ready"));
    act(() => result.current.edit("ws-2", "read", true));
    act(() => {
      void result.current.save("ws-2");
    });
    await waitFor(() =>
      expect(result.current.rows.find((item) => item.id === "ws-2")?.saving).toBe(true),
    );
    act(() => result.current.edit("ws-2", "prompt", true));
    await act(async () => {
      resolveSave({});
    });
    const row = result.current.rows.find((item) => item.id === "ws-2");
    expect(row?.saved.read).toBe(true);
    expect(row?.saved.prompt).toBe(false);
    expect(row?.draft.read).toBe(true);
    expect(row?.draft.prompt).toBe(true);
    expect(row?.saving).toBe(false);
  });
});
