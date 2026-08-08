"use client";

import { useEffect } from "react";
import type { AddPanelOptions, DockviewApi } from "dockview-react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { t } from "@/lib/i18n";
import { focusOrAddPanel } from "@/lib/state/dockview-layout-builders";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  CENTER_GROUP,
  RIGHT_TOP_GROUP,
  isCenterCandidateGroupId,
  type LayoutState,
} from "@/lib/state/layout-manager";

export type ConditionalTodoPanelAction = "add" | "remove" | "none";

/**
 * Pure decision for the Todos panel's runtime presence. Unlike the PR
 * Details conditional panel, there is no per-instance identity to reconcile
 * and no closed-for-session suppression: the `showTodoListPanel` preference
 * is the single authoritative on/off switch (see
 * docs/specs/ui/agent-todo-list-panel.md — "true, unconditional visibility
 * gate"). `settingsLoaded` mirrors PR Details' `reviewsLoaded` guard: don't
 * touch the panel until the real preference value has hydrated, so a
 * cold-loading task with the preference persisted `true` doesn't flash-remove
 * a panel materialized from its saved layout before settings arrive.
 */
export function resolveConditionalTodoPanelAction(params: {
  showTodoListPanel: boolean;
  panelExists: boolean;
  settingsLoaded: boolean;
  isRestoringLayout: boolean;
  isMaximized: boolean;
}): ConditionalTodoPanelAction {
  if (!params.settingsLoaded) return "none";
  if (!params.showTodoListPanel) {
    return params.panelExists ? "remove" : "none";
  }
  if (params.panelExists) return "none";
  if (params.isRestoringLayout || params.isMaximized) return "none";
  return "add";
}

export type TodoPanelPlacement = {
  groupId: string;
  index: number;
};

/** Resolve where a custom Default layout wants the conditional todos tab. */
export function resolveConfiguredTodoPanelPlacement(
  layout: LayoutState | null,
): TodoPanelPlacement | null {
  if (!layout) return null;
  for (const column of layout.columns) {
    for (const group of column.groups) {
      const index = group.panels.findIndex((panel) => panel.id === "todos");
      if (index >= 0 && group.id) return { groupId: group.id, index };
    }
  }
  return null;
}

export type ConditionalTodoPanelOptions = {
  centerGroupId: string;
  configuredPlacement?: TodoPanelPlacement | null;
  isRestoringLayout: boolean;
  isMaximized: boolean;
};

/** Beside Files/Changes in the pinned right column's top group by default —
 *  the user asked for "the right panel" specifically. Falls back to the
 *  center group for layouts with no right column (e.g. compact). */
function resolveTodoPanelTargetPosition(
  api: DockviewApi,
  options: ConditionalTodoPanelOptions,
): NonNullable<AddPanelOptions["position"]> {
  const configured = options.configuredPlacement;
  if (configured && api.groups.some((group) => group.id === configured.groupId)) {
    return { referenceGroup: configured.groupId, index: configured.index };
  }
  if (api.groups.some((group) => group.id === RIGHT_TOP_GROUP)) {
    return { referenceGroup: RIGHT_TOP_GROUP };
  }
  return {
    referenceGroup: isCenterCandidateGroupId(options.centerGroupId)
      ? options.centerGroupId
      : CENTER_GROUP,
  };
}

/**
 * Synchronize the Todos panel's runtime presence with the `showTodoListPanel`
 * preference. The preference is the sole visibility gate; a saved layout's
 * `todos` entry only supplies placement (see
 * `resolveConfiguredTodoPanelPlacement`) and is never rewritten here.
 */
export function syncConditionalTodoPanel(
  api: DockviewApi,
  options: ConditionalTodoPanelOptions & { showTodoListPanel: boolean; settingsLoaded: boolean },
): boolean {
  const panel = api.getPanel("todos");
  const action = resolveConditionalTodoPanelAction({
    showTodoListPanel: options.showTodoListPanel,
    panelExists: !!panel,
    settingsLoaded: options.settingsLoaded,
    isRestoringLayout: options.isRestoringLayout,
    isMaximized: options.isMaximized,
  });

  if (action === "remove") {
    panel?.api.close();
    return true;
  }

  if (action === "add") {
    focusOrAddPanel(
      api,
      {
        id: "todos",
        component: "todos",
        title: t("common:todos"),
        position: resolveTodoPanelTargetPosition(api, options),
      },
      true,
    );
    return true;
  }

  return false;
}

/** Keep the Todos panel in sync with the active task's live Dockview tree. */
export function useSyncTodoPanel() {
  const appStore = useAppStoreApi();
  const showTodoListPanel = useAppStore((state) => state.userSettings.showTodoListPanel);
  const settingsLoaded = useAppStore((state) => state.userSettings.loaded);
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const hasApi = useDockviewStore((state) => !!state.api);
  const isRestoringLayout = useDockviewStore((state) => state.isRestoringLayout);
  const isMaximized = useDockviewStore((state) => state.preMaximizeLayout !== null);
  const centerGroupId = useDockviewStore((state) => state.centerGroupId);
  const userDefaultLayout = useDockviewStore((state) => state.userDefaultLayout);

  useEffect(() => {
    if (!taskId || !sessionId || !workspaceId || !hasApi) return;

    let innerFrame: number | null = null;
    const outerFrame = requestAnimationFrame(() => {
      innerFrame = requestAnimationFrame(() => {
        const live = appStore.getState();
        if (
          live.tasks.activeTaskId !== taskId ||
          live.tasks.activeSessionId !== sessionId ||
          live.workspaces.activeId !== workspaceId
        )
          return;

        const api = useDockviewStore.getState().api;
        if (!api) return;
        const dockview = useDockviewStore.getState();
        syncConditionalTodoPanel(api, {
          showTodoListPanel: live.userSettings.showTodoListPanel,
          settingsLoaded: live.userSettings.loaded,
          centerGroupId: dockview.centerGroupId,
          configuredPlacement: resolveConfiguredTodoPanelPlacement(dockview.userDefaultLayout),
          isRestoringLayout: dockview.isRestoringLayout,
          isMaximized: dockview.preMaximizeLayout !== null,
        });
      });
    });

    return () => {
      cancelAnimationFrame(outerFrame);
      if (innerFrame !== null) cancelAnimationFrame(innerFrame);
    };
  }, [
    appStore,
    centerGroupId,
    hasApi,
    isMaximized,
    isRestoringLayout,
    settingsLoaded,
    sessionId,
    showTodoListPanel,
    taskId,
    userDefaultLayout,
    workspaceId,
  ]);
}
