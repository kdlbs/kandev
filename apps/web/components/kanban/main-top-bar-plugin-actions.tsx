"use client";

import { useMemo } from "react";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import type { MainTopBarSlotProps } from "@/lib/plugins/types";
import type { TaskListingPage } from "@/lib/task-listing/view-navigation";

export type { MainTopBarSlotProps } from "@/lib/plugins/types";

/**
 * The plugin contract predates Threads and names only the two surfaces that
 * existed then. Threads is the same workspace-wide overview the board is, just
 * arranged by conversation, so it reports as "kanban" rather than forcing every
 * installed plugin to handle a third value it has never seen.
 */
export function toPluginTopBarPage(page: TaskListingPage): MainTopBarSlotProps["currentPage"] {
  return page === "tasks" ? "tasks" : "kanban";
}

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
  currentPage: TaskListingPage;
  presentation?: MainTopBarSlotProps["presentation"];
}) {
  const { workspaceId, workspaceLabel, currentPage, presentation = "desktop" } = props;

  const slotProps = useMemo<MainTopBarSlotProps>(
    () => ({
      workspaceId: workspaceId ?? null,
      workspaceLabel,
      currentPage: toPluginTopBarPage(currentPage),
      presentation,
    }),
    [workspaceId, workspaceLabel, currentPage, presentation],
  );

  const content = <PluginSlot name="main-top-bar" slotProps={slotProps} />;
  if (presentation === "desktop") return content;

  return (
    <div
      className="flex shrink-0 items-center gap-2 [&_[data-slot=button]]:!size-8 [&_[data-slot=button]]:!p-0 [&_[data-slot=button]_svg]:!size-4"
      data-testid="mobile-main-top-bar-plugin-actions"
    >
      {content}
    </div>
  );
}
