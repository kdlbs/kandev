import { describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";
import type { WorkspaceState } from "@/lib/state/store";
import {
  buildWorkspaceSaveHandler,
  type SavedState,
  type WorkspaceUpdates,
} from "./workspace-edit-save";
import { isValidFailedRouteAutoRetryMax } from "./workspace-edit-auto-retry-draft";

type Workspace = WorkspaceState["items"][number];

const saved: SavedState = {
  name: "Workspace",
  executorId: "",
  agentProfileId: "",
  idleSuspensionEnabled: false,
  idleTimeoutMinutes: 120,
  midTurnFailoverEnabled: false,
  failedRouteAutoRetryEnabled: false,
  failedRouteAutoRetryMax: 1,
};

const current: Workspace = {
  id: "ws-1",
  name: "Workspace",
  owner_id: "owner",
  acp_idle_suspension_enabled: false,
  acp_idle_timeout_minutes: 120,
  dynamic_midturn_failover_enabled: false,
  dynamic_failed_route_auto_retry_enabled: false,
  dynamic_failed_route_auto_retry_max: 1,
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
};

function saveWith(enabled: boolean, max: string) {
  const sent: WorkspaceUpdates[] = [];
  const setSavedState = vi.fn();
  let items: Workspace[] = [current];
  const handler = buildWorkspaceSaveHandler({
    currentWorkspace: current,
    draft: {
      workspaceNameDraft: "Workspace",
      defaultExecutorId: "",
      defaultAgentProfileId: "",
      idleSuspensionEnabled: false,
      idleTimeoutMinutes: "120",
      midTurnFailoverEnabled: false,
      failedRouteAutoRetryEnabled: enabled,
      failedRouteAutoRetryMax: max,
    },
    savedState: saved,
    isDirty: true,
    setSavedState,
    setCurrentWorkspace: vi.fn(),
    storeApi: {
      getState: () =>
        ({
          workspaces: { items, activeId: current.id },
          setWorkspaces: (next: Workspace[]) => {
            items = next;
          },
        }) as never,
    },
    saveWorkspaceRequest: {
      run: async (_id: string, updates: WorkspaceUpdates) => {
        sent.push(updates);
        return {
          ...current,
          dynamic_failed_route_auto_retry_enabled: enabled,
          dynamic_failed_route_auto_retry_max: Number(max),
        };
      },
    },
    toast: vi.fn(),
    t: ((key: string) => key) as unknown as TFunction,
  });
  return { handler, sent, setSavedState, items: () => items };
}

describe("workspace failed-route auto retry save", () => {
  it("sends the changed toggle and limit and records the saved values", async () => {
    const { handler, sent, setSavedState, items } = saveWith(true, "3");
    await handler();
    expect(sent).toEqual([
      { dynamic_failed_route_auto_retry_enabled: true, dynamic_failed_route_auto_retry_max: 3 },
    ]);
    expect(setSavedState).toHaveBeenCalledWith(
      expect.objectContaining({ failedRouteAutoRetryEnabled: true, failedRouteAutoRetryMax: 3 }),
    );
    expect(items()[0]).toMatchObject({
      dynamic_failed_route_auto_retry_enabled: true,
      dynamic_failed_route_auto_retry_max: 3,
    });
  });

  it("omits both fields when unchanged", async () => {
    const { handler, sent } = saveWith(false, "1");
    await handler();
    expect(sent).toEqual([{}]);
  });

  it("accepts only whole limits from 1 to 3", () => {
    expect(["1", "2", "3", " 2 "].map(isValidFailedRouteAutoRetryMax)).toEqual([
      true,
      true,
      true,
      true,
    ]);
    expect(["0", "4", "1.5", "", "-1", "two"].map(isValidFailedRouteAutoRetryMax)).toEqual([
      false,
      false,
      false,
      false,
      false,
      false,
    ]);
  });
});
