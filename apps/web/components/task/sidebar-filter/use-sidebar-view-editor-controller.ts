"use client";

import { useCallback, type ComponentProps, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useSidebarWorkspaceGuard } from "@/hooks/domains/sidebar/use-sidebar-workspace-guard";
import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";
import type { FilterClause, SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { sidebarViewName } from "@/lib/state/slices/ui/sidebar-view-builtins";
import {
  SidebarViewEditor,
  createSidebarFilterClause,
  createSidebarViewEditorCurrent,
} from "./sidebar-view-editor";
import { useSavedTaskViewDeleteConfirmation } from "@/components/confirmation/use-saved-task-view-delete-confirmation";
import type { SavedTaskViewDeleteTarget } from "@/components/confirmation/use-saved-task-view-delete-confirmation";
import { ViewHeaderRow } from "./view-manager";

type HeaderProps = ComponentProps<typeof ViewHeaderRow>;

export type SidebarViewEditorController = {
  workspaceId: string | null;
  views: SidebarView[];
  activeView: SidebarView | undefined;
  hasDraft: boolean;
  editorProps: ComponentProps<typeof SidebarViewEditor>;
  headerProps: HeaderProps;
  deletion: {
    target: SavedTaskViewDeleteTarget | null;
    anchorRef: RefObject<HTMLButtonElement | null>;
    close: () => void;
  };
  deleteView: (viewId: string) => void;
};

export function useSidebarViewEditorController({
  workspaceId: requestedWorkspaceId,
  isDrawerLayout,
  deleteDensity = "compact",
  renameRequestedViewId,
  onRenameRequestHandled,
}: {
  workspaceId?: string | null;
  isDrawerLayout: boolean;
  deleteDensity?: "compact" | "touch";
  renameRequestedViewId?: string | null;
  onRenameRequestHandled?: (viewId: string) => void;
}): SidebarViewEditorController {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceId = requestedWorkspaceId === undefined ? activeWorkspaceId : requestedWorkspaceId;
  const viewsState = useAppStore((state) => selectSidebarViews(state, workspaceId));
  const views = viewsState.views;
  const activeViewId = viewsState.activeViewId;
  const storedDraft = viewsState.draft;
  const guard = useSidebarWorkspaceGuard();
  const updateDraftAction = useAppStore((state) => state.updateSidebarDraft);
  const saveAsAction = useAppStore((state) => state.saveSidebarDraftAs);
  const saveOverwriteAction = useAppStore((state) => state.saveSidebarDraftOverwrite);
  const discardAction = useAppStore((state) => state.discardSidebarDraft);
  const deleteAction = useAppStore((state) => state.deleteSidebarView);
  const renameAction = useAppStore((state) => state.renameSidebarView);
  const updateDraft = bindWorkspaceAction(guard, updateDraftAction, workspaceId, activeWorkspaceId);
  const saveAs = bindWorkspaceAction(guard, saveAsAction, workspaceId, activeWorkspaceId);
  const saveOverwrite = bindWorkspaceAction(
    guard,
    saveOverwriteAction,
    workspaceId,
    activeWorkspaceId,
  );
  const discard = bindWorkspaceAction(guard, discardAction, workspaceId, activeWorkspaceId);
  const deleteView = bindWorkspaceAction(guard, deleteAction, workspaceId, activeWorkspaceId);
  const renameView = bindWorkspaceAction(guard, renameAction, workspaceId, activeWorkspaceId);
  const deletion = useSavedTaskViewDeleteConfirmation<HTMLButtonElement>(views);
  const activeView = views.find((view) => view.id === activeViewId);
  const current = createSidebarViewEditorCurrent(activeView, storedDraft);
  const hasDraft = !!storedDraft && activeView?.id === storedDraft.baseViewId;
  const onDelete = useCallback(() => {
    if (!activeView) return;
    deletion.request({ id: activeView.id, label: sidebarViewName(activeView, t) });
  }, [activeView, deletion.request, t]);
  const headerProps: HeaderProps = {
    activeView,
    hasDraft,
    canDelete: views.length > 1,
    onSaveOverwrite: saveOverwrite,
    onSaveAs: saveAs,
    onRename: renameView,
    onDiscard: discard,
    onDelete,
    deleteAnchorRef: deletion.anchorRef,
    deleteDensity,
    renameRequestedViewId,
    onRenameRequestHandled,
  };
  const onAddFilter = useCallback(() => {
    updateDraft({ filters: [...current.filters, createSidebarFilterClause()] });
  }, [current.filters, updateDraft]);
  const onChangeClause = useCallback(
    (next: FilterClause) =>
      updateDraft({
        filters: current.filters.map((clause) => (clause.id === next.id ? next : clause)),
      }),
    [current.filters, updateDraft],
  );
  const onRemoveClause = useCallback(
    (id: string) => updateDraft({ filters: current.filters.filter((clause) => clause.id !== id) }),
    [current.filters, updateDraft],
  );

  return {
    workspaceId,
    views,
    activeView,
    hasDraft,
    headerProps,
    editorProps: {
      current,
      isDrawerLayout,
      reorderScopeKey: `${workspaceId ?? ""}:${activeViewId ?? ""}`,
      headerProps,
      onUpdate: updateDraft,
      onAddFilter,
      onChangeClause,
      onRemoveClause,
    },
    deletion,
    deleteView,
  };
}

function bindWorkspaceAction<Args extends unknown[], Result>(
  guard: ReturnType<typeof useSidebarWorkspaceGuard>,
  action: (...args: Args) => Result,
  expectedWorkspaceId: string | null,
  activeWorkspaceId: string | null,
): (...args: Args) => Result | undefined {
  const guardedAction = guard(action);
  return (...args) => {
    if (!expectedWorkspaceId || expectedWorkspaceId !== activeWorkspaceId) return;
    return guardedAction(...args);
  };
}
