import type { ComponentType } from "react";
import { IconHierarchy3, IconLayoutColumns, IconTimeline } from "@tabler/icons-react";
import { SwimlaneKanbanContent } from "@/components/kanban/swimlane-kanban-content";
import { SwimlaneGraph2Content } from "@/components/kanban/swimlane-graph2-content";
import { SwimlaneTaskGraphContent } from "@/components/kanban/swimlane-task-graph-content";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import { getEffectiveViewId, KANBAN_VIEW_MODES, type KanbanViewId } from "./view-modes";

export type ViewContentProps = {
  compactHeight?: boolean;
  /** True when this is the sole rendered workflow lane and should fill the available height. */
  fillHeight?: boolean;
  workflowId: string;
  steps: WorkflowStep[];
  moveTargetSteps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task, opts?: { cascade?: boolean }) => void;
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
  workflows: Array<{ id: string; name: string; taskCount: number | null }>;
  onWorkflowChange: (workflowId: string) => void;
};

export type ViewRegistryEntry = {
  id: string;
  storedValue: string;
  /** Catalog key; resolved wherever the entry is labelled. */
  labelKey: string;
  icon: ComponentType<{ className?: string }>;
  component: ComponentType<ViewContentProps>;
  enabled: boolean;
};

const viewsById: Record<KanbanViewId, ViewRegistryEntry> = {
  kanban: {
    ...KANBAN_VIEW_MODES.kanban,
    labelKey: "kanban:kanban",
    icon: IconLayoutColumns,
    component: SwimlaneKanbanContent as ComponentType<ViewContentProps>,
    enabled: true,
  },
  graph2: {
    ...KANBAN_VIEW_MODES.graph2,
    labelKey: "kanban:pipeline",
    icon: IconTimeline,
    component: SwimlaneGraph2Content,
    enabled: true,
  },
  graph: {
    ...KANBAN_VIEW_MODES.graph,
    labelKey: "kanban:graph",
    icon: IconHierarchy3,
    component: SwimlaneTaskGraphContent,
    enabled: true,
  },
};

export const VIEW_REGISTRY = Object.values(viewsById);

export function getEnabledViews(): ViewRegistryEntry[] {
  return VIEW_REGISTRY.filter((v) => v.enabled);
}

export function getViewByStoredValue(value: string): ViewRegistryEntry | undefined {
  return VIEW_REGISTRY.find((v) => v.storedValue === value);
}

export function getDefaultView(): ViewRegistryEntry {
  return viewsById.kanban;
}

export function getEffectiveView(value: string, isMobile: boolean): ViewRegistryEntry {
  return viewsById[getEffectiveViewId(value, isMobile)];
}
