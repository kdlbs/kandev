import type { SidebarSnapshot } from "./sidebar-view-delta";
import type { SidebarViewStateSetter } from "./sidebar-view-types";
import { t } from "@/lib/i18n";

export function rollbackSidebarState(
  set: SidebarViewStateSetter,
  rollback: SidebarSnapshot,
  after: SidebarSnapshot,
  error?: unknown,
): void {
  set((draft) => {
    draft.sidebarViews.views = rollback.views;
    const activeViewStillExists = rollback.views.some(
      (view) => view.id === draft.sidebarViews.activeViewId,
    );
    if (draft.sidebarViews.activeViewId === after.activeViewId || !activeViewStillExists) {
      draft.sidebarViews.activeViewId = rollback.activeViewId;
    }
    const currentDraft = draft.sidebarViews.draft;
    const draftBaseStillExists =
      !currentDraft || rollback.views.some((view) => view.id === currentDraft.baseViewId);
    if (JSON.stringify(currentDraft) === JSON.stringify(after.draft) || !draftBaseStillExists) {
      draft.sidebarViews.draft = rollback.draft;
    }
    if (error !== undefined) {
      draft.sidebarViews.syncError =
        error instanceof Error ? error.message : t("sidebar:failedToSyncSidebarViews");
    }
  });
}

export function restoreFailedViewMutation(
  set: SidebarViewStateSetter,
  rollback: SidebarSnapshot,
): void {
  set((draft) => {
    draft.sidebarViews.views = rollback.views;
    if (!rollback.views.some((view) => view.id === draft.sidebarViews.activeViewId)) {
      draft.sidebarViews.activeViewId = rollback.activeViewId;
    }
    const currentDraft = draft.sidebarViews.draft;
    if (currentDraft && !rollback.views.some((view) => view.id === currentDraft.baseViewId)) {
      draft.sidebarViews.draft = rollback.draft;
    }
  });
}
