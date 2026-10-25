import { act, renderHook, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { Workspace } from "@/lib/types/http";
import { workspaceId } from "@/lib/types/ids";
import { useWorkspaceClone, canCloneWorkspace } from "./use-workspace-clone";

const action = vi.fn();
const push = vi.fn();
const toast = vi.fn();
let store = createAppStore();
vi.mock("@/app/actions/workspaces", () => ({
  cloneWorkspaceAction: (...args: unknown[]) => action(...args),
}));
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: () => store }));
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
