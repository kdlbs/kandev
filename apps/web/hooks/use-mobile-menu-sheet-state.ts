"use client";

import { useRef } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { linkToTaskOverview, linkToTasks } from "@/lib/links";
import type {
  MobileColumnsSection,
  MobileDisplayOptionsProps,
  MobileMenuSheetProps,
} from "@/components/kanban/mobile-menu-sheet";
import type { TasksListDisplayOptions } from "@/components/kanban/mobile-menu-task-list-options";

type MobileMenuViewChangeInput = {
  currentPage: "kanban" | "tasks";
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
  const goToBoard = () =>
    router.push(linkToTaskOverview({ workspaceId, workflowId: activeWorkflowId ?? undefined }));
  return (value: string) => {
    if (!value) return;
    if (value === "list") {
      onViewModeChange("list");
      if (currentPage !== "tasks") router.push(linkToTasks(workspaceId));
    } else if (value === "kanban") {
      onViewModeChange("kanban");
      if (currentPage !== "kanban") goToBoard();
    } else if (value === "pipeline" && !isMobile) {
      onViewModeChange("pipeline");
      if (currentPage !== "kanban") goToBoard();
    } else {
      return;
    }
    onOpenChange(false);
  };
}

type BuildMobileDisplayOptionsInput = Omit<
  MobileDisplayOptionsProps,
  "showTaskDetails" | "showWorkflow" | "tasksListOptions" | "columnsSection"
> & {
  currentPage: "kanban" | "tasks";
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
  currentPage: "kanban" | "tasks";
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
  const viewValue = currentPage === "tasks" ? "list" : effectiveTaskListingView;
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
