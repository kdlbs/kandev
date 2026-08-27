"use client";

import { useEffect } from "react";
import type { AddPanelOptions, DockviewApi } from "dockview-react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { buildTodoItems } from "@/hooks/use-processed-messages";
import { t } from "@/lib/i18n";
import type { Message } from "@/lib/types/http";
import type { TodoEntry } from "@/lib/state/slices/session-runtime/types";
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
 * docs/specs/ui/requirements/agent-todo-list-panel.md — "true, unconditional visibility
 * gate"). The "only pin when not empty" sub-option (`onlyPinWhenNotEmpty`)
 * gates only the automatic add: while it is on, an absent panel is not added
 * until the active session's todo list is non-empty (`todoListNotEmpty`).
 * It never removes an existing panel, and removal stays gated solely on the
 * master preference. `settingsLoaded` mirrors PR Details' `reviewsLoaded`
 * guard: don't touch the panel until the real preference value has hydrated,
 * so a cold-loading task with the preference persisted `true` doesn't
 * flash-remove a panel materialized from its saved layout before settings
 * arrive.
 */
export function resolveConditionalTodoPanelAction(params: {
  showTodoListPanel: boolean;
  onlyPinWhenNotEmpty: boolean;
  todoListNotEmpty: boolean;
  hasActiveSession: boolean;
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
  if (!params.hasActiveSession) return "none";
  if (params.isRestoringLayout || params.isMaximized) return "none";
  if (params.onlyPinWhenNotEmpty && !params.todoListNotEmpty) return "none";
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

/** Full runtime inputs to {@link syncConditionalTodoPanel}: the placement
 *  options plus the visibility preferences and the hydrated settings flag. */
export type SyncConditionalTodoPanelOptions = ConditionalTodoPanelOptions & {
  showTodoListPanel: boolean;
  onlyPinWhenNotEmpty: boolean;
  todoListNotEmpty: boolean;
  hasActiveSession: boolean;
  settingsLoaded: boolean;
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
 * preference (and the "only pin when not empty" sub-option). The preference
 * is the sole visibility gate; a saved layout's `todos` entry only supplies
 * placement (see `resolveConfiguredTodoPanelPlacement`) and is never
 * rewritten here.
 */
export function syncConditionalTodoPanel(
  api: DockviewApi,
  options: SyncConditionalTodoPanelOptions,
): boolean {
  const panel = api.getPanel("todos");
  const action = resolveConditionalTodoPanelAction({
    showTodoListPanel: options.showTodoListPanel,
    onlyPinWhenNotEmpty: options.onlyPinWhenNotEmpty,
    todoListNotEmpty: options.todoListNotEmpty,
    hasActiveSession: options.hasActiveSession,
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

const EMPTY_MESSAGES: Message[] = [];

/** True when the session's todo list is non-empty, using the exact two-source
 *  fallback the Todos panel content uses: live `sessionTodos.bySessionId`
 *  entries first (an empty array falls through), then the latest persisted
 *  `todo`-type message via `buildTodoItems`. `useSyncTodoPanel` uses the
 *  streaming-optimized `createTodoMessagePresenceTracker` for the messages
 *  half on its hot path; this canonical form is kept as the testable
 *  contract so the fallback can't drift from the panel content. */
export function todoListNotEmptyForSession(
  liveTodos: readonly TodoEntry[] | undefined,
  messages: Message[],
): boolean {
  return (liveTodos?.length ?? 0) > 0 || buildTodoItems(messages).length > 0;
}

/** Keep the Todos panel in sync with the active task's live Dockview tree. */
export function useSyncTodoPanel() {
  const appStore = useAppStoreApi();
  const showTodoListPanel = useAppStore((state) => state.userSettings.showTodoListPanel);
  const onlyPinWhenNotEmpty = useAppStore(
    (state) => state.userSettings.showTodoListPanelOnlyWhenNotEmpty,
  );
  const settingsLoaded = useAppStore((state) => state.userSettings.loaded);
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const hasApi = useDockviewStore((state) => !!state.api);
  const isRestoringLayout = useDockviewStore((state) => state.isRestoringLayout);
  const isMaximized = useDockviewStore((state) => state.preMaximizeLayout !== null);
  const centerGroupId = useDockviewStore((state) => state.centerGroupId);
  const userDefaultLayout = useDockviewStore((state) => state.userDefaultLayout);
  // The "not empty" predicate mirrors the Todos panel's two-source fallback:
  // live `sessionTodos.bySessionId` entries first, then the latest persisted
  // `todo`-type message. Messages are fetched by the always-mounted chat
  // panel, so subscribing to the store slices here adds no fetch side effects.
  // Selectors must return stable references (the stored array or `undefined`),
  // never a fresh `[]`, or zustand re-renders on every store read and React
  // throws "maximum update depth exceeded". The slices double as the effect's
  // change signal, so the predicate is computed exactly once per sync — in
  // the dispatch callback, from the dispatch-time snapshot.
  const liveTodos = useAppStore((state) =>
    sessionId ? state.sessionTodos.bySessionId[sessionId] : undefined,
  );
  const messages = useAppStore((state) =>
    sessionId ? (state.messages.bySession[sessionId] ?? EMPTY_MESSAGES) : EMPTY_MESSAGES,
  );

  useEffect(() => {
    // Run even for sessionless tasks (no `!sessionId` guard): a saved layout
    // can materialize a `todos` tab for a task without a session, and the
    // master preference must still be able to remove it (spec: "the
    // preference is a true, unconditional visibility gate"). A null session
    // never enables an add (`hasActiveSession` gates the add path in the
    // resolver).
    if (!taskId || !workspaceId || !hasApi) return;

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
        // Recompute the predicate from the dispatch-time snapshot rather than
        // a render-time value: a WS todo event landing between the render and
        // this rAF tick would otherwise make the decision one frame stale
        // (the tab would be missed until the next effect run). The predicate
        // only matters when the sub-option is on, so skip the O(n) transcript
        // scan entirely otherwise; a null session is treated as an empty list.
        syncConditionalTodoPanel(api, {
          showTodoListPanel: live.userSettings.showTodoListPanel,
          onlyPinWhenNotEmpty: live.userSettings.showTodoListPanelOnlyWhenNotEmpty,
          // A sessionless task never auto-adds a Todos tab (there is nothing
          // to show); it only ever gets removal handling. The predicate is
          // computed only when the sub-option is on, skipping the transcript
          // scan otherwise, and the live slice short-circuits the message
          // scan while the session has live todo entries.
          hasActiveSession: sessionId !== null,
          todoListNotEmpty:
            live.userSettings.showTodoListPanelOnlyWhenNotEmpty && sessionId
              ? todoListNotEmptyForSession(
                  live.sessionTodos.bySessionId[sessionId],
                  live.messages.bySession[sessionId] ?? EMPTY_MESSAGES,
                )
              : false,
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
    liveTodos,
    messages,
    onlyPinWhenNotEmpty,
    settingsLoaded,
    sessionId,
    showTodoListPanel,
    taskId,
    userDefaultLayout,
    workspaceId,
  ]);
}
