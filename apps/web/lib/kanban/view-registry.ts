import type { ComponentType } from "react";
import { IconLayoutColumns, IconTimeline } from "@tabler/icons-react";
import { SwimlaneKanbanContent } from "@/components/kanban/swimlane-kanban-content";
import { SwimlaneGraph2Content } from "@/components/kanban/swimlane-graph2-content";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { MoveTaskError } from "@/lib/kanban/move-task-error";

export type ViewContentProps = {
  workflowId: string;
  steps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  onMoveError?: (error: MoveTaskError) => void;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  showMaximizeButton?: boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
  mobileWorkflowNavigation?: MobileWorkflowNavigation;
};

export type MobileWorkflowNavigation = {
  activeWorkflowId: string;
  workflows: Array<{ id: string; name: string; taskCount: number }>;
  onWorkflowChange: (workflowId: string) => void;
};

export type ViewRegistryEntry = {
  id: string;
  storedValue: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
  component: ComponentType<ViewContentProps>;
  enabled: boolean;
};

export const VIEW_REGISTRY: ViewRegistryEntry[] = [
  {
    id: "kanban",
    storedValue: "",
    label: "Kanban",
    icon: IconLayoutColumns,
    component: SwimlaneKanbanContent as ComponentType<ViewContentProps>,
    enabled: true,
  },
  {
    id: "graph2",
    storedValue: "graph2",
    label: "Pipeline",
    icon: IconTimeline,
    component: SwimlaneGraph2Content as ComponentType<ViewContentProps>,
    enabled: true,
  },
];

export function getEnabledViews(): ViewRegistryEntry[] {
  return VIEW_REGISTRY.filter((v) => v.enabled);
}

export function getViewByStoredValue(value: string): ViewRegistryEntry | undefined {
  return VIEW_REGISTRY.find((v) => v.storedValue === value);
}

export function getDefaultView(): ViewRegistryEntry {
  return VIEW_REGISTRY[0];
}

export function getEffectiveView(value: string, isMobile: boolean): ViewRegistryEntry {
  if (isMobile) return getDefaultView();
  return getViewByStoredValue(value) ?? getDefaultView();
}
