import { createSidebarWorkspaceState, mapSidebarWorkspaces } from "./sidebar-workspace-state";
import { requestUserSettingsUpdateWithRetry } from "@/lib/user-settings-sync";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";
import type { UISliceState } from "./types";
import type { SidebarViewStateSetter as ImmerSet } from "./sidebar-view-types";
import type { SidebarSnapshot } from "./sidebar-view-delta";
import {
  admitSidebarServerSnapshot,
  getSidebarWriteJournal,
  snapshotSidebar,
  type SidebarWriteJournal,
  type SidebarWriteKind,
} from "./sidebar-view-journal";
import { t } from "@/lib/i18n";

type SidebarActionState = Pick<UISliceState, "sidebarViews">;
type SidebarViewPatch = Omit<
  NonNullable<UserSettingsUpdatePayload["sidebar_view_state"]>,
  "workspace_id"
>;

const workspaceBySetter = new WeakMap<ImmerSet, string>();
const sidebarSettingsQueues = new WeakMap<ImmerSet, Promise<void>>();

export function bindSidebarViewSettingsWorkspace(set: ImmerSet, workspaceId: string) {
  workspaceBySetter.set(set, workspaceId);
}

export function waitForSidebarSettings(set: ImmerSet): Promise<void> {
  return sidebarSettingsQueues.get(set) ?? Promise.resolve();
}

export function trackSidebarSettingsWrite(set: ImmerSet, write: Promise<void>) {
  sidebarSettingsQueues.set(set, write);
}

export function admitDeferredSidebarState(
  journal: SidebarWriteJournal,
  get: () => SidebarActionState,
) {
  const server = get().sidebarViews.deferredServerState;
  if (server) admitSidebarServerSnapshot(journal, snapshotSidebar(server));
}

async function reconcileUncertainSidebarCreate(
  journal: SidebarWriteJournal,
  get: () => SidebarActionState,
  workspaceId: string,
) {
  if (!journal.reconcileUncertainCreate) return;
  const response = await fetchUserSettings({ cache: "no-store" });
  const settings = response.settings;
  const server = settings.sidebar_views_by_workspace?.[workspaceId];
  const snapshot = server
    ? snapshotSidebar(
        mapSidebarWorkspaces({ [workspaceId]: server }, {}, settings.revision)[workspaceId],
      )
    : {
        ...snapshotSidebar(createSidebarWorkspaceState()),
        serverRevision: settings.revision,
      };
  admitSidebarServerSnapshot(journal, snapshot);
  admitDeferredSidebarState(journal, get);
  journal.reconcileUncertainCreate = false;
}

export function enqueueSidebarSettingsSync(
  set: ImmerSet,
  get: () => SidebarActionState,
  payload: () => SidebarViewPatch | null,
  after: () => SidebarSnapshot,
  kind: SidebarWriteKind,
): Promise<boolean> {
  const workspaceId = workspaceBySetter.get(set);
  if (!workspaceId) {
    // i18n-exempt: programmer invariant diagnostic; this setter is registered before use.
    return Promise.reject(new Error("sidebar set not bound to a workspace"));
  }
  const send = async () => {
    const journal = getSidebarWriteJournal(set);
    await reconcileUncertainSidebarCreate(journal, get, workspaceId);
    const patch = payload();
    if (!patch) return undefined;
    const update =
      kind === "compaction" || kind === "draft_save" || kind === "selection" || kind === "create"
        ? updateUserSettings
        : requestUserSettingsUpdateWithRetry;
    return update({
      sidebar_view_state: { workspace_id: workspaceId, ...patch },
    });
  };
  const previous = sidebarSettingsQueues.get(set);
  const write = previous ? previous.then(send) : send();
  return write.then((response) => {
    if (!response) return false;
    const server = response.settings.sidebar_views_by_workspace?.[workspaceId];
    if (!server && (kind === "selection" || kind === "draft_save"))
      throw new Error(t("sidebar:failedToSyncSidebarViews"));
    const journal = getSidebarWriteJournal(set);
    admitDeferredSidebarState(journal, get);
    admitSidebarServerSnapshot(
      journal,
      server
        ? snapshotSidebar(
            mapSidebarWorkspaces({ [workspaceId]: server }, {}, response.settings.revision)[
              workspaceId
            ],
          )
        : { ...after(), views: kind !== "local" ? after().views : journal.confirmed!.views },
    );
    if (!server) return true;
    set((state) => {
      const mapped = mapSidebarWorkspaces(
        { [workspaceId]: server },
        { [workspaceId]: state.sidebarViews },
        response.settings.revision,
      );
      Object.assign(state.sidebarViews, mapped[workspaceId]);
    });
    return true;
  });
}
