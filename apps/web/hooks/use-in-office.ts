"use client";

import { useFeature } from "@/hooks/domains/features/use-feature";
import { useWorkspaceScope, type WorkspaceMode } from "@/components/workspace-scope-provider";

/**
 * Office-vs-kanban mode for the current workspace scope.
 *
 * This used to read the pathname: "in Office" meant "on an `/office` route".
 * That made mode a property of the URL, so visiting any shared surface —
 * Settings, Stats, a task page, an integration dashboard — silently ejected an
 * Office user back to kanban chrome, and going Home from there switched their
 * active workspace as a side effect. Mode is a property of the workspace you
 * have selected; the URL follows it, never the other way round.
 *
 * Returns `"unknown"` until the workspace list has hydrated. Callers that paint
 * mode-specific chrome should hold on that rather than assume kanban, or the
 * sidebar renders one mode and then swaps.
 */
export function useOfficeModeState(): WorkspaceMode {
  const officeEnabled = useFeature("office");
  const { mode } = useWorkspaceScope();
  // A build with the feature off has no `/api/v1/office/*` routes at all, so an
  // office workspace degrades to kanban chrome rather than to a broken surface.
  if (!officeEnabled) return mode === "unknown" ? "unknown" : "kanban";
  return mode;
}

/**
 * True while the active workspace is an Office workspace.
 *
 * Kept as a boolean for the surfaces that only choose between two renderings.
 * An unresolved workspace reads as `false` here — use `useOfficeModeState()`
 * where the difference between "kanban" and "not known yet" matters.
 */
export function useInOffice(): boolean {
  return useOfficeModeState() === "office";
}
