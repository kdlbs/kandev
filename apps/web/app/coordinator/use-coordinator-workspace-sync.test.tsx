import type { ReactNode } from "react";
import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { WorkspaceItem } from "@/lib/state/slices/workspace/selectors";
import type { AppState } from "@/lib/state/store";
import { useSyncActiveWorkspaceToRoute } from "./use-coordinator-workspace-sync";

function workspace(id: string): WorkspaceItem {
  return {
    id,
    name: id,
    owner_id: "owner-1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function makeWrapper(items: WorkspaceItem[], initialActiveId: string | null) {
  let captured: StoreApi<AppState> | null = null;
  function Capture() {
    captured = useAppStoreApi();
    return null;
  }
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider initialState={{ workspaces: { items, activeId: initialActiveId } }}>
        <Capture />
        {children}
      </StateProvider>
    );
  }
  return { Wrapper, getStore: () => captured };
}

// The coordinator route names its workspace in the URL. Deep-linking,
// bookmarking, or cold-loading it for a workspace that isn't the globally
// active one left the task/workflow data hooks (keyed off `activeId` via
// `useEnsureWorkspaceWorkflows`) permanently empty with no error, because
// they never fetch for a workspace the store hasn't marked active.
describe("useSyncActiveWorkspaceToRoute", () => {
  it("switches the active workspace to the route's workspace when they differ", () => {
    const { Wrapper, getStore } = makeWrapper([workspace("ws-1"), workspace("ws-2")], "ws-2");
    renderHook(() => useSyncActiveWorkspaceToRoute("ws-1"), { wrapper: Wrapper });
    expect(getStore()?.getState().workspaces.activeId).toBe("ws-1");
    expect(getStore()?.getState().workspaces.activeIdRevision).toBe(1);
  });

  it("does nothing when the route's workspace is already active", () => {
    const { Wrapper, getStore } = makeWrapper([workspace("ws-1")], "ws-1");
    renderHook(() => useSyncActiveWorkspaceToRoute("ws-1"), { wrapper: Wrapper });
    expect(getStore()?.getState().workspaces.activeId).toBe("ws-1");
    expect(getStore()?.getState().workspaces.activeIdRevision ?? 0).toBe(0);
  });

  it("does not activate a workspace id that is not in the known workspace list", () => {
    const { Wrapper, getStore } = makeWrapper([workspace("ws-2")], "ws-2");
    renderHook(() => useSyncActiveWorkspaceToRoute("ws-unknown"), { wrapper: Wrapper });
    expect(getStore()?.getState().workspaces.activeId).toBe("ws-2");
    expect(getStore()?.getState().workspaces.activeIdRevision ?? 0).toBe(0);
  });

  it("re-syncs when the route's workspace id changes across a re-render", () => {
    const { Wrapper, getStore } = makeWrapper([workspace("ws-1"), workspace("ws-2")], "ws-1");
    const { rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) => useSyncActiveWorkspaceToRoute(workspaceId),
      { wrapper: Wrapper, initialProps: { workspaceId: "ws-1" } },
    );
    expect(getStore()?.getState().workspaces.activeId).toBe("ws-1");
    rerender({ workspaceId: "ws-2" });
    expect(getStore()?.getState().workspaces.activeId).toBe("ws-2");
  });
});
