import type { TFunction } from "i18next";
import type { useToast } from "@/components/toast-provider";
import type { WorkspaceState } from "@/lib/state/slices";

type Workspace = WorkspaceState["items"][number];

export type SavedState = {
  name: string;
  executorId: string;
  agentProfileId: string;
  idleSuspensionEnabled: boolean;
  idleTimeoutMinutes: number;
};

export type WorkspaceUpdates = {
  name?: string;
  default_executor_id?: string;
  default_agent_profile_id?: string;
  acp_idle_suspension_enabled?: boolean;
  acp_idle_timeout_minutes?: number;
};

export type WorkspaceDraftState = {
  workspaceNameDraft: string;
  defaultExecutorId: string;
  defaultAgentProfileId: string;
  idleSuspensionEnabled: boolean;
  idleTimeoutMinutes: string;
};

type SaveRequestLike = {
  run: (id: string, updates: WorkspaceUpdates) => Promise<Workspace>;
};

type WorkspaceSaveHandlerOptions = {
  currentWorkspace: Workspace;
  draft: WorkspaceDraftState;
  savedState: SavedState;
  isDirty: boolean;
  setSavedState: (state: SavedState) => void;
  setCurrentWorkspace: (update: (previous: Workspace) => Workspace) => void;
  workspaces: Workspace[];
  setWorkspaces: (items: Workspace[]) => void;
  saveWorkspaceRequest: SaveRequestLike;
  toast: ReturnType<typeof useToast>["toast"];
  t: TFunction;
};

function buildWorkspaceUpdates(draft: WorkspaceDraftState, saved: SavedState): WorkspaceUpdates {
  const updates: WorkspaceUpdates = {};
  const name = draft.workspaceNameDraft.trim();
  if (name !== saved.name) updates.name = name;
  if (draft.defaultExecutorId !== saved.executorId) {
    updates.default_executor_id = draft.defaultExecutorId;
  }
  if (draft.defaultAgentProfileId !== saved.agentProfileId) {
    updates.default_agent_profile_id = draft.defaultAgentProfileId;
  }
  if (draft.idleSuspensionEnabled !== saved.idleSuspensionEnabled) {
    updates.acp_idle_suspension_enabled = draft.idleSuspensionEnabled;
  }
  const timeoutMinutes = Number(draft.idleTimeoutMinutes);
  if (timeoutMinutes !== saved.idleTimeoutMinutes) {
    updates.acp_idle_timeout_minutes = timeoutMinutes;
  }
  return updates;
}

export function buildWorkspaceSaveHandler({
  currentWorkspace,
  draft,
  savedState,
  isDirty,
  setSavedState,
  setCurrentWorkspace,
  workspaces,
  setWorkspaces,
  saveWorkspaceRequest,
  toast,
  t,
}: WorkspaceSaveHandlerOptions) {
  return async () => {
    if (!isDirty) return;
    try {
      const updates = buildWorkspaceUpdates(draft, savedState);
      const updated = await saveWorkspaceRequest.run(currentWorkspace.id, updates);
      setCurrentWorkspace((previous) => ({ ...previous, ...updated }));
      setSavedState({
        name: updated.name ?? draft.workspaceNameDraft.trim(),
        executorId: updated.default_executor_id ?? "",
        agentProfileId: updated.default_agent_profile_id ?? "",
        idleSuspensionEnabled: updated.acp_idle_suspension_enabled ?? draft.idleSuspensionEnabled,
        idleTimeoutMinutes: updated.acp_idle_timeout_minutes ?? Number(draft.idleTimeoutMinutes),
      });
      setWorkspaces(
        workspaces.map((workspace) =>
          workspace.id === updated.id
            ? {
                ...workspace,
                name: updated.name,
                default_executor_id: updated.default_executor_id ?? null,
                default_environment_id: updated.default_environment_id ?? null,
                default_agent_profile_id: updated.default_agent_profile_id ?? null,
                acp_idle_suspension_enabled: updated.acp_idle_suspension_enabled,
                acp_idle_timeout_minutes: updated.acp_idle_timeout_minutes,
              }
            : workspace,
        ),
      );
    } catch (error) {
      toast({
        title: t("workspaces:failedToSaveWorkspace"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
      throw error;
    }
  };
}
