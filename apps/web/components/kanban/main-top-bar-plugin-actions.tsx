"use client";

import { useMemo } from "react";
import { PluginSlot } from "@/components/plugins/plugin-slot";

/**
 * Props forwarded to every plugin component registered for the `main-top-bar`
 * slot (`registry.registerComponent("main-top-bar", Component)`). This is the
 * default app top bar's right-hand cluster on the Home / Kanban / Tasks views,
 * beside the CPU/DB metrics and the view/display controls — the place for
 * at-a-glance status or an action a plugin wants to surface app-wide, as
 * opposed to the per-session `chat-top-bar` slot.
 *
 * The bar is not scoped to a task, so no task/session ids are provided; the
 * context a plugin gets is the active workspace and which listing view is
 * showing.
 */
export type MainTopBarSlotProps = {
  /** Workspace the top bar is currently showing, or null on the global home. */
  workspaceId: string | null;
  /** Human-readable label of that workspace, when known. */
  workspaceLabel?: string;
  /** Which listing the top bar belongs to. */
  currentPage: "kanban" | "tasks";
};

/**
 * Plugin extension point in the default app top bar (Home / Kanban / Tasks),
 * rendered alongside the first-party controls (metrics, view toggle, display
 * menu, health indicator). Renders every plugin component registered for the
 * `main-top-bar` slot (each isolated behind its own error boundary via
 * `PluginSlot`) and forwards the active workspace and current view as
 * `slotProps`.
 */
export function MainTopBarPluginActions(props: {
  workspaceId?: string;
  workspaceLabel?: string;
  currentPage: "kanban" | "tasks";
}) {
  const { workspaceId, workspaceLabel, currentPage } = props;

  const slotProps = useMemo<MainTopBarSlotProps>(
    () => ({ workspaceId: workspaceId ?? null, workspaceLabel, currentPage }),
    [workspaceId, workspaceLabel, currentPage],
  );

  return <PluginSlot name="main-top-bar" slotProps={slotProps} />;
}
