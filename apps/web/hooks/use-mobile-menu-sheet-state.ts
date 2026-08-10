"use client";

import { useRef, useState } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useMobileStepsSection } from "@/hooks/use-mobile-steps-section";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { linkToTaskOverview, linkToTasks } from "@/lib/links";
import type {
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
  "showTaskDetails" | "showWorkflow" | "tasksListOptions"
> & {
  currentPage: "kanban" | "tasks";
  isMobile: boolean;
  tasksListOptions?: TasksListDisplayOptions;
};

function buildMobileDisplayOptions({
  currentPage,
  isMobile,
  tasksListOptions,
  ...rest
}: BuildMobileDisplayOptionsInput): MobileDisplayOptionsProps {
  return {
    ...rest,
    showTaskDetails: currentPage === "tasks",
    showWorkflow: !isMobile || currentPage !== "kanban",
    tasksListOptions: isMobile && currentPage === "tasks" ? tasksListOptions : undefined,
  };
}

/** All of `MobileMenuSheet`'s data wiring — settings, view-change routing, and the phone Steps section — kept out of the component so its own body stays render-only. */
export function useMobileMenuSheetState({
  open,
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
  const [improveOpen, setImproveOpen] = useState(false);
  const { isMobile, breakpoint } = useResponsiveBreakpoint();
  const {
    workflows,
    activeWorkflowId,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryId,
    enablePreviewOnClick,
    tasksListShowDetails,
    eligibleWorkflows,
    snapshots,
    hiddenWorkflowStepIds,
    onWorkflowChange,
    onRepositoryChange,
    onTogglePreviewOnClick,
    onToggleTasksListShowDetails,
    onToggleStepVisibility,
    effectiveTaskListingView,
    onViewModeChange,
  } = useKanbanDisplaySettings();
  const stepsSection = useMobileStepsSection({
    open,
    breakpoint,
    isMobile,
    currentPage: currentPage ?? "kanban",
    eligibleWorkflows,
    snapshots,
    hiddenWorkflowStepIds,
    onToggleStepVisibility,
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
    stepsSection,
    currentPage: currentPage ?? "kanban",
    isMobile,
    tasksListOptions,
  });
  const focusMenu = (event: Event) => {
    event.preventDefault();
    contentRef.current?.focus({ preventScroll: true });
  };
  const openImproveKandev = () => {
    onOpenChange(false);
    requestAnimationFrame(() => setImproveOpen(true));
  };

  return {
    contentRef,
    router,
    improveOpen,
    setImproveOpen,
    isMobile,
    viewValue,
    handleViewChange,
    displayOptions,
    focusMenu,
    openImproveKandev,
  };
}
