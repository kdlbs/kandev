"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Drawer, DrawerContent } from "@kandev/ui/drawer";
import { KanbanHeader } from "@/components/kanban/kanban-header";
import { useAppStore } from "@/components/state-provider";
import { PluginTaskLinkActionSurfaceProvider } from "@/components/task/task-session-sidebar-link-actions";
import { SidebarViewChips } from "@/components/task/sidebar-filter/sidebar-view-chips";
import {
  MobileTaskList,
  type MobileTaskListProps,
} from "@/components/task/mobile/mobile-task-list";
import {
  MobileTaskMoveOptionsSurface,
  useMobileTaskMoveOptions,
} from "@/components/task/mobile/mobile-task-move-options";
import {
  TaskSwitcherDialogs,
  type MobileSubtaskTarget,
} from "@/components/task/mobile/session-task-switcher-sheet-dialogs";
import {
  useMobileTaskLinking,
  useTaskSheetSelectionController,
} from "@/components/task/mobile/session-task-switcher-sheet";
import {
  useSheetActions,
  useSheetData,
} from "@/components/task/mobile/session-task-switcher-sheet-hooks";
import { useTaskReadStatus } from "@/components/task/mobile/session-task-switcher-sheet-read-status";
import { useMobileTaskRename } from "@/components/task/mobile/use-mobile-task-rename";
import { useSidebarTaskEdit } from "@/components/task/task-session-sidebar-edit";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useMobileListResults } from "@/hooks/use-mobile-list-results";
import { MobileListToolbar } from "./mobile-list-toolbar";
import { MobileTasksActions } from "./mobile-tasks-actions";
import type { MobileTaskStep } from "./mobile-tasks-create-dialog";
import { useTaskListFacets } from "@/hooks/use-task-list-facets";
import type { MobileListFacetSettings } from "@/lib/tasks/mobile-list-query";

const EMPTY_STEPS: MobileTaskStep[] = [];
const NOOP = () => {};

type MobileTasksListSurfaceProps = {
  workspaceId: string | null;
  view: ReturnType<typeof useEffectiveSidebarView>;
  data: ReturnType<typeof useSheetData>;
  actions: ReturnType<typeof useSheetActions>;
  rename: ReturnType<typeof useMobileTaskRename>;
  edit: ReturnType<typeof useSidebarTaskEdit>;
  linking: ReturnType<typeof useMobileTaskLinking>;
  moveOptions: ReturnType<typeof useMobileTaskMoveOptions>;
  taskLoadError: string | null;
  retryTaskLoad?: () => void;
  createSubtask: (taskId: string, title: string) => void;
  scrollRef: MutableRefObject<HTMLDivElement | null>;
  facetValues: ReturnType<typeof useTaskListFacets>["values"];
  facetSortKey: string | null;
  facetGroupKey: string | null;
  resultGroup: ReturnType<typeof useSheetData>["page"]["resultView"]["group"];
  resultSort: ReturnType<typeof useSheetData>["page"]["resultView"]["sort"];
  hasLinkSortOverride: boolean;
  hasLinkGroupOverride: boolean;
  hasResultRefinements: boolean;
  onClearResultRefinements: () => void;
};

function MobileTasksListSurface(props: MobileTasksListSurfaceProps) {
  const { t } = useTranslation();
  return (
    <PluginTaskLinkActionSurfaceProvider>
      <MobileTaskList {...buildMobileTaskListProps(props, t)} />
    </PluginTaskLinkActionSurfaceProvider>
  );
}

function buildMobileTaskListProps(
  props: MobileTasksListSurfaceProps,
  t: ReturnType<typeof useTranslation>["t"],
): MobileTaskListProps {
  const { workspaceId, view, data, actions, rename, edit, linking, moveOptions } = props;
  return {
    workspaceId,
    view,
    tasks: data.tasksWithRepositories,
    workflows: data.workflows,
    stepsByWorkflowId: data.stepsByWorkflowId,
    activeTaskId: data.activeTaskId,
    selectedTaskId: data.selectedTaskId,
    onSelectTask: actions.handleSelectTask,
    onMoveToStep: moveOptions.handleMove,
    onRequestMoveOptions: moveOptions.handleRequest,
    onBeforeMoveOptionsOpen: NOOP,
    onEditTask: edit.handleEditTask,
    onRenameTask: rename.handleRenameTask,
    onCreateSubtask: props.createSubtask,
    onArchiveTask: actions.handleArchiveTask,
    onDeleteTask: actions.handleDeleteTask,
    onDetachTask: actions.handleDetachTask,
    archivingTaskId: actions.archivingTaskId,
    isArchiving: actions.isArchiving,
    onNestTask: actions.handleNestTask,
    onLinkPullRequest: linking.taskListHandlers.onLinkPullRequest,
    onLinkIssue: linking.taskListHandlers.onLinkIssue,
    onLinkMergeRequest: linking.taskListHandlers.onLinkMergeRequest,
    onLinkJiraTicket: linking.taskListHandlers.onLinkJiraTicket,
    onLinkLinearIssue: linking.taskListHandlers.onLinkLinearIssue,
    onLinkSentryIssue: linking.taskListHandlers.onLinkSentryIssue,
    deletingTaskId: actions.deletingTaskId,
    isLoading:
      data.tasksLoading ||
      (data.workspaceContextPending && data.tasksWithRepositories.length === 0),
    loadError: props.taskLoadError,
    onRetryLoad: props.retryTaskLoad,
    retryLabel: t("sidebar:retry"),
    pageEntries: data.pageEntries,
    page: data.page.response,
    pagePending: data.page.requestedPage !== null,
    pageTransitioning: data.page.isDisclosureTransition,
    pageError: data.page.error,
    pageCanRetry: data.page.canRetry,
    emptyState: buildMobileListEmptyState(props, t),
    facetValues: props.facetValues,
    facetSortKey: props.facetSortKey,
    facetGroupKey: props.facetGroupKey,
    resultGroup: props.resultGroup,
    resultSort: props.resultSort,
    hasLinkSortOverride: props.hasLinkSortOverride,
    hasLinkGroupOverride: props.hasLinkGroupOverride,
    onPageChange: data.page.goToPage,
    onPageRetry: data.page.retry,
    scrollContainerRef: props.scrollRef,
  };
}

function buildMobileListEmptyState(
  props: MobileTasksListSurfaceProps,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (!props.hasResultRefinements) return undefined;
  return (
    <div
      className="px-3 py-3 text-xs text-muted-foreground"
      data-testid="mobile-list-filter-empty-state"
    >
      <p>{t("tasks:noMatchingTasks")}</p>
      <Button
        type="button"
        variant="link"
        className="mt-1 min-h-11 cursor-pointer px-0"
        onClick={props.onClearResultRefinements}
        data-testid="mobile-list-clear-results"
      >
        {t("tasks:clearFilters")}
      </Button>
    </div>
  );
}

function MobileTasksDialogsAndMoveOptions({
  workspaceId,
  workflowId,
  data,
  actions,
  rename,
  edit,
  linking,
  subtaskTarget,
  setSubtaskTarget,
  moveOptions,
}: {
  workspaceId: string | null;
  workflowId: string | null;
  data: ReturnType<typeof useSheetData>;
  actions: ReturnType<typeof useSheetActions>;
  rename: ReturnType<typeof useMobileTaskRename>;
  edit: ReturnType<typeof useSidebarTaskEdit>;
  linking: ReturnType<typeof useMobileTaskLinking>;
  subtaskTarget: MobileSubtaskTarget | null;
  setSubtaskTarget: (target: MobileSubtaskTarget | null) => void;
  moveOptions: ReturnType<typeof useMobileTaskMoveOptions>;
}) {
  return (
    <>
      <TaskSwitcherDialogs
        dialogOpen={false}
        onDialogOpenChange={NOOP}
        workspaceId={workspaceId}
        workflowId={workflowId}
        data={data}
        actions={actions}
        rename={rename}
        edit={edit}
        linking={linking}
        subtaskTarget={subtaskTarget}
        onSubtaskTargetChange={setSubtaskTarget}
      />
      {moveOptions.moveOptionsStep && moveOptions.request && (
        <Drawer
          open
          onOpenChange={(open) => {
            if (!open) moveOptions.handleClose();
          }}
        >
          <DrawerContent className="h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] !max-h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))]">
            <MobileTaskMoveOptionsSurface
              presentation="drawer"
              step={moveOptions.moveOptionsStep}
              isMoving={moveOptions.isMoving}
              onBack={moveOptions.handleClose}
              onSubmit={moveOptions.handleSubmit}
            />
          </DrawerContent>
        </Drawer>
      )}
    </>
  );
}

function useMobileListPageModel(active: boolean) {
  const display = useKanbanDisplaySettings();
  const workspaceId = display.activeWorkspaceId;
  const view = useEffectiveSidebarView(workspaceId);
  const results = useMobileListResults(workspaceId, view, display.repositories);
  const data = useSheetData(workspaceId, results.queryOverrides, active);
  const { facets, values: facetValues } = useTaskListFacets(
    active ? data.tasksWithRepositories : [],
    active ? workspaceId : null,
  );
  const facetOptions = useMemo(
    () => facets.map((facet) => ({ value: facet.key, label: facet.label })),
    [facets],
  );
  const facetSortKey = resolveFacetOverride(results.facetSortOverride, facetOptions);
  const facetGroupKey = resolveFacetOverride(results.facetGroupOverride, facetOptions);
  const mobileListFacets = useMemo<MobileListFacetSettings>(
    () => ({
      options: facetOptions,
      sort: results.facetSortOverride,
      group: results.facetGroupOverride,
      onSortChange: results.setFacetSortOverride,
      onGroupChange: results.setFacetGroupOverride,
      onReset: results.resetFacetOverrides,
    }),
    [
      facetOptions,
      results.facetGroupOverride,
      results.facetSortOverride,
      results.resetFacetOverrides,
      results.setFacetGroupOverride,
      results.setFacetSortOverride,
    ],
  );
  return {
    display,
    workspaceId,
    view,
    results,
    data,
    facetValues,
    facetSortKey,
    facetGroupKey,
    mobileListFacets,
  };
}

function resolveFacetOverride(
  facetKey: string | null,
  options: Array<{ value: string; label: string }>,
) {
  if (!facetKey) return null;
  return options.some((option) => option.value === facetKey) ? facetKey : null;
}

type MobileListPageModel = ReturnType<typeof useMobileListPageModel>;

function useMobileTaskPageInteractions(active: boolean, model: MobileListPageModel) {
  const { workspaceId, data, display, results } = model;
  const isSearchOpen = useAppStore((state) => state.mobileKanban.isSearchOpen);
  const setSearchOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);
  const selection = useTaskSheetSelectionController();
  const keepPageOpen = useCallback(() => {}, []);
  const actions = useSheetActions(
    workspaceId,
    keepPageOpen,
    selection,
    undefined,
    data.tasksWithRepositories,
  );
  const rename = useMobileTaskRename();
  const edit = useSidebarTaskEdit(data.allTasks);
  const linking = useMobileTaskLinking(workspaceId, data.allTasks);
  const { taskLoadError, retryTaskLoad } = useTaskReadStatus(data);
  const listScrollRef = useRef<HTMLDivElement | null>(null);
  const previousWorkspaceId = useRef(workspaceId);
  const [createOpen, setCreateOpen] = useState(false);
  const [subtaskTarget, setSubtaskTarget] = useState<MobileSubtaskTarget | null>(null);
  const moveOptions = useMobileTaskMoveOptions({
    open: true,
    activeTaskId: data.activeTaskId,
    stepsByWorkflowId: data.stepsByWorkflowId,
  });
  const activeSteps = useAppStore((state) =>
    state.kanban.workflowId === display.activeWorkflowId ? state.kanban.steps : EMPTY_STEPS,
  );
  const createSubtask = useCallback(
    (taskId: string, title: string) => {
      setSubtaskTarget({
        id: taskId,
        title,
        task: data.allTasks.find((task) => task.id === taskId),
      });
    },
    [data.allTasks],
  );
  const refreshAfterCreate = useCallback(async () => {
    data.page.refresh();
  }, [data.page]);

  useEffect(() => {
    if (!active) return;
    setSearchOpen(results.hasLinkScope);
  }, [active, results.hasLinkScope, setSearchOpen]);

  useEffect(() => {
    const previousId = previousWorkspaceId.current;
    previousWorkspaceId.current = workspaceId;
    if (!active || !previousId || !workspaceId || previousId === workspaceId) return;
    results.selectWorkspace();
    setSearchOpen(false);
  }, [active, results.selectWorkspace, setSearchOpen, workspaceId]);

  return {
    isSearchOpen,
    actions,
    rename,
    edit,
    linking,
    taskLoadError,
    retryTaskLoad,
    listScrollRef,
    createOpen,
    setCreateOpen,
    subtaskTarget,
    setSubtaskTarget,
    moveOptions,
    activeSteps,
    createSubtask,
    refreshAfterCreate,
  };
}

type MobileTaskPageInteractions = ReturnType<typeof useMobileTaskPageInteractions>;

export function MobileTasksPageClient({ active = true }: { active?: boolean }) {
  const model = useMobileListPageModel(active);
  const interactions = useMobileTaskPageInteractions(active, model);
  return <MobileTasksPageLayout active={active} model={model} interactions={interactions} />;
}

function MobileTasksPageLayout({
  active,
  model,
  interactions,
}: {
  active: boolean;
  model: MobileListPageModel;
  interactions: MobileTaskPageInteractions;
}) {
  const { display, workspaceId, data } = model;
  const { actions, moveOptions, createOpen, setCreateOpen, activeSteps, refreshAfterCreate } =
    interactions;
  return (
    <>
      {active && <MobileListResultsSurface model={model} interactions={interactions} />}
      {workspaceId && (
        <MobileTasksActions
          key="mobile-task-actions"
          workspaceId={workspaceId}
          workflowId={display.activeWorkflowId}
          steps={activeSteps}
          open={createOpen}
          showFab={active}
          onOpenChange={setCreateOpen}
          onCreated={refreshAfterCreate}
        />
      )}
      <MobileTasksDialogsAndMoveOptions
        key="mobile-task-dialogs"
        workspaceId={workspaceId}
        workflowId={display.activeWorkflowId}
        data={data}
        actions={actions}
        rename={interactions.rename}
        edit={interactions.edit}
        linking={interactions.linking}
        subtaskTarget={interactions.subtaskTarget}
        setSubtaskTarget={interactions.setSubtaskTarget}
        moveOptions={moveOptions}
      />
    </>
  );
}

function MobileListResultsSurface({
  model,
  interactions,
}: {
  model: MobileListPageModel;
  interactions: MobileTaskPageInteractions;
}) {
  const {
    workspaceId,
    view,
    results,
    data,
    facetValues,
    facetSortKey,
    facetGroupKey,
    mobileListFacets,
  } = model;
  const { isSearchOpen, listScrollRef } = interactions;
  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-background">
      <KanbanHeader
        workspaceId={workspaceId ?? undefined}
        currentPage="tasks"
        searchQuery={results.searchQuery}
        onSearchChange={results.setSearchQuery}
        onSearchClose={results.clearSearchAndFilters}
        isSearchLoading={data.tasksLoading}
        mobileListFacets={mobileListFacets}
      />
      <div className="flex min-h-10 shrink-0 items-center px-4" data-testid="mobile-list-view-tabs">
        <SidebarViewChips workspaceId={workspaceId} onViewSelect={results.selectView} />
      </div>
      <MobileListToolbar
        key={workspaceId ?? "no-workspace"}
        open={isSearchOpen}
        searchQuery={results.searchQuery}
        onSearchChange={results.setSearchQuery}
        isSearchLoading={data.tasksLoading}
        resultFilters={results.resultFilters}
        filterDraft={results.filterDraft}
        hasLinkScope={results.hasLinkScope}
        onClearLinkScope={results.clearAllResults}
        onRemoveResultFilter={results.removeResultFilter}
        onBeginFilterEdit={results.beginFilterEdit}
        onSetFilterDraft={results.setFilterDraft}
        onCancelFilterEdit={results.cancelFilterEdit}
        onApplyFilterEdit={results.applyFilterEdit}
        onClearFilterDraft={results.clearFilterDraft}
        workspaceId={workspaceId}
      />
      <main className="flex min-h-0 flex-1 flex-col">
        <div
          ref={listScrollRef}
          className="min-h-0 flex-1 overflow-y-auto px-4 pb-[calc(6rem+env(safe-area-inset-bottom,0px))]"
          data-testid="mobile-tasks-list-scroll"
        >
          <MobileTasksListSurface
            workspaceId={workspaceId}
            view={view}
            data={data}
            actions={interactions.actions}
            rename={interactions.rename}
            edit={interactions.edit}
            linking={interactions.linking}
            moveOptions={interactions.moveOptions}
            taskLoadError={interactions.taskLoadError}
            retryTaskLoad={interactions.retryTaskLoad}
            createSubtask={interactions.createSubtask}
            scrollRef={listScrollRef}
            facetValues={facetValues}
            facetSortKey={facetSortKey}
            facetGroupKey={facetGroupKey}
            resultGroup={data.page.resultView.group}
            resultSort={data.page.resultView.sort}
            hasLinkSortOverride={results.linkScope?.sort !== undefined}
            hasLinkGroupOverride={results.linkScope?.group !== undefined}
            hasResultRefinements={Boolean(
              results.searchQuery ||
              results.hasResultFilters ||
              results.linkScope?.query ||
              results.linkScope?.filters?.length,
            )}
            onClearResultRefinements={results.clearAllResults}
          />
        </div>
      </main>
    </div>
  );
}
