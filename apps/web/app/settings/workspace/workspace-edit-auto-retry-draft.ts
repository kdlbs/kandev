import { useState } from "react";
import type { WorkspaceState } from "@/lib/state/store";
import type { FailedRouteAutoRetryControls } from "@/components/settings/workspaces/workspace-dynamic-routing-card";
import type { SavedState } from "./workspace-edit-save";

type Workspace = WorkspaceState["items"][number];

/** Accepted automatic failed-route retry limits: a whole number from 1 to 3. */
export function isValidFailedRouteAutoRetryMax(value: string): boolean {
  return /^[1-3]$/.test(value.trim());
}

/** The saved automatic failed-route retry policy of a workspace. */
export function savedFailedRouteAutoRetryState(
  workspace: Workspace,
): Pick<SavedState, "failedRouteAutoRetryEnabled" | "failedRouteAutoRetryMax"> {
  return {
    failedRouteAutoRetryEnabled: workspace.dynamic_failed_route_auto_retry_enabled ?? false,
    failedRouteAutoRetryMax: workspace.dynamic_failed_route_auto_retry_max ?? 1,
  };
}

/** Draft of the workspace's automatic failed-route retry policy. */
export function useWorkspaceFailedRouteAutoRetryDraft(workspace: Workspace, saved: SavedState) {
  const [enabled, setEnabled] = useState(
    workspace.dynamic_failed_route_auto_retry_enabled ?? false,
  );
  const [max, setMax] = useState(String(workspace.dynamic_failed_route_auto_retry_max ?? 1));
  const maxValid = isValidFailedRouteAutoRetryMax(max);
  const isDirty =
    enabled !== saved.failedRouteAutoRetryEnabled ||
    !maxValid ||
    Number(max) !== saved.failedRouteAutoRetryMax;
  // Turning the policy off drops an invalid limit draft, so it cannot block
  // saving a setting the user no longer edits.
  const changeEnabled = (next: boolean) => {
    if (!next && !maxValid) setMax(String(saved.failedRouteAutoRetryMax));
    setEnabled(next);
  };
  const reset = () => {
    setEnabled(saved.failedRouteAutoRetryEnabled);
    setMax(String(saved.failedRouteAutoRetryMax));
  };
  return { enabled, setEnabled: changeEnabled, max, setMax, maxValid, isDirty, reset };
}

export type FailedRouteAutoRetryDraft = ReturnType<typeof useWorkspaceFailedRouteAutoRetryDraft>;

/** The settings card's controls for a draft, with per-field dirty state. */
export function failedRouteAutoRetryControls(
  draft: FailedRouteAutoRetryDraft,
  saved: SavedState,
): FailedRouteAutoRetryControls {
  return {
    enabled: draft.enabled,
    max: draft.max,
    maxValid: draft.maxValid,
    enabledIsDirty: draft.enabled !== saved.failedRouteAutoRetryEnabled,
    maxIsDirty: draft.max.trim() !== String(saved.failedRouteAutoRetryMax),
    onEnabledChange: draft.setEnabled,
    onMaxChange: draft.setMax,
  };
}
