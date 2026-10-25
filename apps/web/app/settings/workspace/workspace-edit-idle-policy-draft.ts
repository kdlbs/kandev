import { useState } from "react";
import type { WorkspaceState } from "@/lib/state/slices";
import type { SavedState } from "./workspace-edit-save";

type Workspace = WorkspaceState["items"][number];

export function useWorkspaceIdlePolicyDraft(workspace: Workspace, saved: SavedState) {
  const [enabled, setEnabled] = useState(workspace.acp_idle_suspension_enabled ?? false);
  const [timeoutMinutes, setTimeoutMinutes] = useState(
    String(workspace.acp_idle_timeout_minutes ?? 120),
  );
  const timeoutValue = Number(timeoutMinutes);
  const timeoutValid = /^\d+$/.test(timeoutMinutes) && timeoutValue > 0;
  const isDirty =
    enabled !== saved.idleSuspensionEnabled ||
    (timeoutValid && timeoutValue !== saved.idleTimeoutMinutes) ||
    !timeoutValid;

  return { enabled, setEnabled, timeoutMinutes, setTimeoutMinutes, timeoutValid, isDirty };
}
