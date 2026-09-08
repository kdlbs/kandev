"use client";

import { useRef } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type {
  MobileColumnsSection,
  MobileDisplayOptionsProps,
  MobileMenuSheetProps,
} from "@/components/kanban/mobile-menu-sheet";
import type { TasksListDisplayOptions } from "@/components/kanban/mobile-menu-task-list-options";
import {
  resolveTaskListingNavigation,
  type TaskListingPage,
} from "@/lib/task-listing/view-navigation";

type MobileMenuViewChangeInput = {
  currentPage: TaskListingPage;
  workspaceId?: string;
  activeWorkflowId: string | null;
  isMobile: boolean;
  onViewModeChange: (mode: string) => void;
  onOpenChange: (open: boolean) => void;
  router: ReturnType<typeof useRouter>;
};

function buildMobileMenuViewChange({
  currentPage,
  workspaceId,
  activeWorkflowId,
  isMobile,
  onViewModeChange,
  onOpenChange,
  router,
}: MobileMenuViewChangeInput) {
  return (value: string) => {
    const next = resolveTaskListingNavigation({
      view: value,
      currentPage,
      workspaceId,
      workflowId: activeWorkflowId,
      allowPipeline: !isMobile,
    });
    if (!next) return;
    onViewModeChange(next.view);
    if (next.href) router.push(next.href);
    onOpenChange(false);
  };
}

/** The page a phone is on pins the segment, exactly as it does on desktop. */
function resolveMobileMenuViewValue(currentPage: TaskListingPage, effectiveView: string): string {
  if (currentPage === "tasks") return "list";
  return currentPage === "threads" ? "threads" : effectiveView;
}

type BuildMobileDisplayOptionsInput = Omit<
  MobileDisplayOptionsProps,
  | "showTaskDetails"
  | "showWorkflow"
  | "showRepository"
  | "showPreviewPanel"
  | "tasksListOptions"
  | "columnsSection"
> & {
  currentPage: TaskListingPage;
  isMobile: boolean;
  tasksListOptions?: TasksListDisplayOptions;
};

function buildMobileDisplayOptions({
  currentPage,
  isMobile,
  tasksListOptions,
  columnsSection,
  ...rest
}: BuildMobileDisplayOptionsInput & {
  columnsSection: MobileColumnsSection | null;
}): MobileDisplayOptionsProps {
  return {
    ...rest,
    showTaskDetails: currentPage === "tasks",
    showWorkflow: !isMobile || currentPage !== "kanban",
    showRepository: currentPage !== "threads",
    showPreviewPanel: currentPage !== "threads",
    tasksListOptions: isMobile && currentPage === "tasks" ? tasksListOptions : undefined,
    columnsSection,
  };
}

/**
 * The phone's Columns block, scoped to the workflow the board reported as
 * focused. Off the phone kanban it is null — there the swimlane header owns
 * the control, and rendering both would duplicate its testids.
 */
function buildColumnsSection({
  isMobile,
  currentPage,
  focusedWorkflowId,
  eligibleWorkflows,
  snapshots,
  hiddenWorkflowStepIds,
  onToggleStepVisibility,
  workflowIdsWithAutoHideEmptySteps,
  onToggleAutoHideEmpty,
}: {
  isMobile: boolean;
  currentPage: TaskListingPage;
  focusedWorkflowId: string | null;
  eligibleWorkflows: Array<{ id: string; name: string }>;
  snapshots: Record<string, { steps?: Array<{ id: string; title: string; position: number }> }>;
  hiddenWorkflowStepIds: Record<string, string[]>;
  onToggleStepVisibility: (workflowId: string, stepId: string) => void;
  workflowIdsWithAutoHideEmptySteps: string[];
  onToggleAutoHideEmpty: (workflowId: string) => void;
}): MobileColumnsSection | null {
  if (!isMobile || currentPage !== "kanban" || !focusedWorkflowId) return null;
  const workflow = eligibleWorkflows.find((item) => item.id === focusedWorkflowId);
  if (!workflow) return null;
  return {
    workflowId: workflow.id,
    workflowName: workflow.name,
    steps: snapshots[workflow.id]?.steps ?? [],
    hiddenStepIds: hiddenWorkflowStepIds[workflow.id] ?? [],
    onToggle: onToggleStepVisibility,
    autoHideEmpty: workflowIdsWithAutoHideEmptySteps.includes(workflow.id),
    onToggleAutoHide: onToggleAutoHideEmpty,
  };
}

/** All of `MobileMenuSheet`'s data wiring — settings, view-change routing — kept out of the component so its own body stays render-only. */
export function useMobileMenuSheetState({
  onOpenChange,
  workspaceId,
  currentPage,
  tasksListOptions,
}: Pick<
  MobileMenuSheetProps,
  "open" | "onOpenChange" | "workspaceId" | "currentPage" | "tasksListOptions"
>) {
  const contentRef = useRef<HTMLDivElement | null>(null);
  const router = useRouter();
  const { isMobile } = useResponsiveBreakpoint();
  const {
    workflows,
    activeWorkflowId,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryId,
    enablePreviewOnClick,
    tasksListShowDetails,
    onWorkflowChange,
    onRepositoryChange,
    onTogglePreviewOnClick,
    onToggleTasksListShowDetails,
    effectiveTaskListingView,
    onViewModeChange,
    eligibleWorkflows,
    snapshots,
    hiddenWorkflowStepIds,
    onToggleStepVisibility,
    workflowIdsWithAutoHideEmptySteps,
    onToggleAutoHideEmpty,
  } = useKanbanDisplaySettings();
  const focusedWorkflowId = useAppStore((state) => state.mobileKanban.focusedWorkflowId);
  const columnsSection = buildColumnsSection({
    isMobile,
    currentPage: currentPage ?? "kanban",
    focusedWorkflowId,
    eligibleWorkflows,
    snapshots,
    hiddenWorkflowStepIds,
    onToggleStepVisibility,
    workflowIdsWithAutoHideEmptySteps,
    onToggleAutoHideEmpty,
  });
  const repositoryValue = allRepositoriesSelected ? "all" : (selectedRepositoryId ?? "all");
  const viewValue = resolveMobileMenuViewValue(currentPage ?? "kanban", effectiveTaskListingView);
  const handleViewChange = buildMobileMenuViewChange({
    currentPage: currentPage ?? "kanban",
    workspaceId,
    activeWorkflowId,
    isMobile,
    onViewModeChange,
    onOpenChange,
    router,
  });
  const displayOptions = buildMobileDisplayOptions({
    activeWorkflowId,
    workflows,
    onWorkflowChange,
    repositoryValue,
    repositories,
    repositoriesLoading,
    onRepositoryChange,
    enablePreviewOnClick,
    onTogglePreviewOnClick,
    tasksListShowDetails,
    onToggleTasksListShowDetails,
    currentPage: currentPage ?? "kanban",
    isMobile,
    columnsSection,
    tasksListOptions,
  });
  const focusMenu = (event: Event) => {
    event.preventDefault();
    contentRef.current?.focus({ preventScroll: true });
  };

  return { contentRef, isMobile, viewValue, handleViewChange, displayOptions, focusMenu };
}
