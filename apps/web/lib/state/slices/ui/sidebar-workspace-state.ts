import { compareUserSettingsRevisions } from "@/lib/settings/user-settings-revision";
import type { SidebarWorkspaceStateApi } from "@/lib/types/http-user-settings";
import { fromApiSidebarView, fromApiSidebarDraft } from "./sidebar-view-wire";
import { migrateView, migrateSidebarViewDraft } from "./ui-slice";
import type { SidebarSliceState } from "./sidebar-view-types";
import { DEFAULT_VIEW, createDefaultSidebarView } from "./sidebar-view-builtins";

export type SidebarWorkspaceSource = {
  workspaces: { activeId: string | null };
  sidebarViewsByWorkspace?: Record<string, SidebarSliceState>;
};

const EMPTY_SIDEBAR: SidebarSliceState = {
  views: [],
  activeViewId: "",
  draft: null,
  syncError: null,
};
const DEFAULT_SIDEBAR: SidebarSliceState = {
  views: [DEFAULT_VIEW],
  activeViewId: DEFAULT_VIEW.id,
  draft: null,
  syncError: null,
};

export function createSidebarWorkspaceState(): SidebarSliceState {
  return {
    ...DEFAULT_SIDEBAR,
    views: [createDefaultSidebarView(DEFAULT_VIEW.id, DEFAULT_VIEW.name)],
  };
}

export function selectSidebarViews(
  state: SidebarWorkspaceSource,
  workspaceId: string | null = state.workspaces.activeId,
): SidebarSliceState {
  const id = workspaceId;
  if (!id) return EMPTY_SIDEBAR;
  const workspace = state.sidebarViewsByWorkspace?.[id];
  return workspace?.views.length ? workspace : DEFAULT_SIDEBAR;
}

function mapWorkspaceState(
  wire: SidebarWorkspaceStateApi,
  current: SidebarSliceState | undefined,
  revision: number | null | undefined,
): SidebarSliceState {
  const mappedViews = wire.views.map(fromApiSidebarView).map(migrateView);
  const views = mappedViews.length > 0 ? mappedViews : DEFAULT_SIDEBAR.views;
  const activeViewId = views.some((view) => view.id === wire.active_view_id)
    ? wire.active_view_id
    : (views[0]?.id ?? "");
  const draft =
    mappedViews.length > 0 && wire.draft && wire.draft.base_view_id === activeViewId
      ? migrateSidebarViewDraft(fromApiSidebarDraft(wire.draft))
      : null;
  return {
    views,
    activeViewId,
    draft,
    syncError: current?.syncError ?? null,
    serverRevision: revision,
  };
}

export function mapSidebarWorkspaces(
  incoming: Record<string, SidebarWorkspaceStateApi> | undefined,
  current: Record<string, SidebarSliceState> = {},
  revision?: number | null,
): Record<string, SidebarSliceState> {
  if (incoming == null) return current;
  const result = Object.fromEntries(
    Object.entries(incoming).map(([id, wire]) => {
      const order = compareUserSettingsRevisions(revision, current[id]?.serverRevision);
      if (current[id] && order !== null && order <= 0) return [id, current[id]];
      const server = mapWorkspaceState(wire, current[id], revision);
      if (current[id]?.syncPending)
        return [id, { ...current[id], serverRevision: revision, deferredServerState: server }];
      return [id, server];
    }),
  );
  for (const [id, entry] of Object.entries(current)) {
    if (id in result) continue;
    const older = compareUserSettingsRevisions(revision, entry.serverRevision) === -1;
    if (entry.syncPending || older) result[id] = entry;
  }
  return result;
}
