import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { UserSettingsUpdatedPayload } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { mapUserSettingsData } from "@/lib/ssr/user-settings";
import { fromApiSidebarDraft, fromApiSidebarView } from "@/lib/state/slices/ui/sidebar-view-wire";
import { fromApiThreadDraft, fromApiThreadView } from "@/lib/state/slices/ui/thread-view-wire";
import { normalizeThreadViews } from "@/lib/state/slices/ui/thread-view-builtins";
import type { ThreadViewSnapshot } from "@/lib/state/slices/ui/thread-view-types";
import { migrateSidebarViewDraft, migrateView } from "@/lib/state/slices/ui/ui-slice";
import { compareUserSettingsRevisions } from "@/lib/settings/user-settings-revision";
import {
  mapAgentProfileRecentUseApiRecords,
  mergeAgentProfileRecentUseState,
} from "@/lib/agent-profile-recent-use";

export function registerUsersHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "user.agent_profile_recent_use.updated": (message) => {
      store.setState((state) => ({
        ...state,
        agentProfileRecentUse: mergeAgentProfileRecentUseState(
          state.agentProfileRecentUse,
          mapAgentProfileRecentUseApiRecords([message.payload]),
        ),
      }));
    },
    "user.settings.updated": (message) => {
      store.setState((state) => {
        const order = compareUserSettingsRevisions(
          message.payload.revision,
          state.userSettings.revision,
        );
        if (order !== null && order <= 0) return state;
        return {
          ...state,
          sidebarViews: buildSidebarViewsState(state, message.payload),
          threadViews: buildThreadViewsState(state, message.payload),
          sidebarTaskPrefs: buildSidebarTaskPrefsState(state, message.payload),
          userSettings: buildUserSettingsState(state, message.payload),
        };
      });
    },
  };
}

function buildUserSettingsState(state: AppState, payload: UserSettingsUpdatedPayload) {
  return mapUserSettingsData(payload, state.userSettings);
}

function buildSidebarTaskPrefsState(state: AppState, payload: UserSettingsUpdatedPayload) {
  if (!payload.sidebar_task_prefs) return state.sidebarTaskPrefs;
  if (state.sidebarTaskPrefs.syncPending) return state.sidebarTaskPrefs;
  return {
    pinnedTaskIds: payload.sidebar_task_prefs.pinned_task_ids ?? [],
    orderedTaskIds: payload.sidebar_task_prefs.ordered_task_ids ?? [],
    subtaskOrderByParentId: payload.sidebar_task_prefs.subtask_order_by_parent_id ?? {},
    syncError: state.sidebarTaskPrefs.syncError,
  };
}

function buildSidebarViewsState(state: AppState, payload: UserSettingsUpdatedPayload) {
  const views = (payload.sidebar_views ?? []).map(fromApiSidebarView).map(migrateView);
  const draft = parseSidebarDraftForViews(state, payload);
  if (views.length === 0) return { ...state.sidebarViews, draft };
  const collapsedById = new Map(
    state.sidebarViews.views.map((view) => [view.id, view.collapsedGroups]),
  );
  const mergedViews = views.map((view) => ({
    ...view,
    collapsedGroups: collapsedById.get(view.id) ?? view.collapsedGroups,
  }));
  const activeViewId =
    payload.sidebar_active_view_id &&
    mergedViews.some((v) => v.id === payload.sidebar_active_view_id)
      ? payload.sidebar_active_view_id
      : state.sidebarViews.activeViewId;
  return {
    ...state.sidebarViews,
    views: mergedViews,
    activeViewId: mergedViews.some((v) => v.id === activeViewId) ? activeViewId : mergedViews[0].id,
    draft,
  };
}

function parseSidebarDraftForViews(state: AppState, payload: UserSettingsUpdatedPayload) {
  if (payload.sidebar_draft === undefined) return state.sidebarViews.draft;
  if (payload.sidebar_draft === null) return null;
  return migrateSidebarViewDraft(fromApiSidebarDraft(payload.sidebar_draft));
}

function buildThreadViewsState(state: AppState, payload: UserSettingsUpdatedPayload) {
  const serverState = projectThreadViewsState(state, payload);
  if (!serverState) return state.threadViews;
  // Keep the complete authoritative projection for a pending write. The
  // revision is advanced by the common settings handler, so dropping this
  // payload would make it impossible to reconcile it after a write failure.
  if (state.threadViews.syncPending) {
    return { ...state.threadViews, deferredServerState: serverState };
  }
  return { ...state.threadViews, ...serverState, deferredServerState: null };
}

function projectThreadViewsState(
  state: AppState,
  payload: UserSettingsUpdatedPayload,
): ThreadViewSnapshot | null {
  const hasViews = payload.thread_views !== undefined;
  const hasActive = payload.thread_active_view_id !== undefined;
  const hasDraft = payload.thread_view_draft !== undefined;
  if (!hasViews && !hasActive && !hasDraft) return null;

  const views = hasViews
    ? normalizeThreadViews(payload.thread_views?.map(fromApiThreadView))
    : state.threadViews.views;
  const activeViewId = resolveThreadActiveViewId(
    state.threadViews.activeViewId,
    payload.thread_active_view_id,
    views,
  );
  const draft = resolveThreadDraft(state.threadViews.draft, payload.thread_view_draft);
  return { views, activeViewId, draft };
}

function resolveThreadActiveViewId(
  currentId: string,
  payloadId: string | undefined,
  views: ReturnType<typeof normalizeThreadViews>,
): string {
  if (payloadId && views.some((view) => view.id === payloadId)) return payloadId;
  if (views.some((view) => view.id === currentId)) return currentId;
  return views[0].id;
}

function resolveThreadDraft(
  currentDraft: AppState["threadViews"]["draft"],
  payloadDraft: UserSettingsUpdatedPayload["thread_view_draft"],
) {
  if (payloadDraft === undefined) return currentDraft;
  if (payloadDraft === null) return null;
  return fromApiThreadDraft(payloadDraft);
}
