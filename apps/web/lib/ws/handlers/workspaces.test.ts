import { describe, expect, it } from "vitest";
import { createStore } from "zustand/vanilla";
import type { AppState } from "@/lib/state/store";
import type { WorkspacePayload } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";
import { registerWorkspacesHandlers } from "./workspaces";

const NOW = "2026-01-01T00:00:00.000Z";

function makeStore(overrides: Partial<AppState>) {
  return createStore<AppState>(() => overrides as AppState);
}

function makeMessage(
  payload: WorkspacePayload,
): BackendMessage<"workspace.deleted", WorkspacePayload> {
  return {
    type: "notification",
    action: "workspace.deleted",
    payload,
    timestamp: NOW,
  };
}

describe("workspace.deleted legacy kanban mirror", () => {
  it("does not mutate kanban when the active workspace is deleted", () => {
    const store = makeStore({
      workspaces: {
        activeId: "workspace-1",
        items: [
          {
            id: "workspace-1",
            name: "Deleted",
            description: null,
            owner_id: "",
            default_executor_id: null,
            default_environment_id: null,
            default_agent_profile_id: null,
            default_config_agent_profile_id: null,
            created_at: NOW,
            updated_at: NOW,
          },
          {
            id: "workspace-2",
            name: "Next",
            description: null,
            owner_id: "",
            default_executor_id: null,
            default_environment_id: null,
            default_agent_profile_id: null,
            default_config_agent_profile_id: null,
            created_at: NOW,
            updated_at: NOW,
          },
        ],
      },
      workflows: { activeId: "wf-1" },
    } as unknown as AppState);

    const handlers = registerWorkspacesHandlers(store);
    handlers["workspace.deleted"]!(makeMessage({ id: "workspace-1", name: "Deleted" }));

    expect(store.getState().workspaces.activeId).toBe("workspace-2");
    expect(store.getState().workflows.activeId).toBeNull();
    expect("kanban" in store.getState()).toBe(false);
  });
});
