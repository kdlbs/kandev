import { describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";
import type { WorkspaceState } from "@/lib/state/store";
import {
  buildWorkspaceSaveHandler,
  type SavedState,
  type WorkspaceUpdates,
} from "./workspace-edit-save";

type Workspace = WorkspaceState["items"][number];

const saved: SavedState = {
  name: "Workspace",
  executorId: "",
  agentProfileId: "",
  idleSuspensionEnabled: false,
  idleTimeoutMinutes: 120,
  midTurnFailoverEnabled: false,
};

const current: Workspace = {
  id: "ws-1",
  name: "Workspace",
  owner_id: "owner",
  acp_idle_suspension_enabled: false,
  acp_idle_timeout_minutes: 120,
  dynamic_midturn_failover_enabled: false,
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
};

function saveWith(midTurnFailoverEnabled: boolean) {
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
      midTurnFailoverEnabled,
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
        return { ...current, dynamic_midturn_failover_enabled: midTurnFailoverEnabled };
      },
    },
    toast: vi.fn(),
    t: ((key: string) => key) as unknown as TFunction,
  });
  return { handler, sent, setSavedState, items: () => items };
}

describe("workspace mid-turn failover save", () => {
  it("sends only the changed toggle and records the saved value", async () => {
    const { handler, sent, setSavedState, items } = saveWith(true);
    await handler();
    expect(sent).toEqual([{ dynamic_midturn_failover_enabled: true }]);
    expect(setSavedState).toHaveBeenCalledWith(
      expect.objectContaining({ midTurnFailoverEnabled: true }),
    );
    expect(items()[0]?.dynamic_midturn_failover_enabled).toBe(true);
  });

  it("omits the field when the toggle is unchanged", async () => {
    const { handler, sent } = saveWith(false);
    await handler();
    expect(sent).toEqual([{}]);
  });
});
