import { useAppStore } from "@/components/state-provider";
import { normalizeCopilotItemId, type CopilotItemRef } from "@/lib/coordinator/copilot-id";

/** The route kinds that count as a workspace page. */
export type WorkspacePageRoute =
  | { kind: "kanban" }
  | { kind: "needsYouInbox" }
  | { kind: "taskDetail"; taskId: string };

export type PageContext = {
  ref: CopilotItemRef;
  /** The identifier or workflow name; the only field besides the id that leaves the page. */
  label: string;
  /** The workflow the page belongs to, when known. */
  workflowId: string | null;
};

/** The page's identity for the chip: a query or hash change keeps it. */
export function pageRouteKey(route: WorkspacePageRoute, activeWorkflowId: string | null): string {
  if (route.kind === "taskDetail") return `taskDetail:${route.taskId}`;
  if (route.kind === "kanban") return `kanban:${activeWorkflowId ?? ""}`;
  return "needsYouInbox:";
}

/** The workspace's active workflow id, as the route key needs it. */
export function useActiveWorkflowId(): string | null {
  return useAppStore((s) => s.workflows.activeId);
}

function useTaskContext(taskId: string | null, workspaceId: string): PageContext | null {
  const task = useAppStore((s) =>
    taskId ? s.kanban.tasks.find((candidate) => candidate.id === taskId) : undefined,
  );
  if (!taskId || !task || task.workspaceId !== workspaceId || !task.identifier) return null;
  return {
    ref: { kind: "task", id: taskId },
    label: normalizeCopilotItemId(task.identifier),
    workflowId: task.workflowId ?? null,
  };
}

function useWorkflowContext(enabled: boolean, workspaceId: string): PageContext | null {
  const workflow = useAppStore((s) => {
    const id = s.workflows.activeId;
    return enabled && id ? s.workflows.items.find((item) => item.id === id) : undefined;
  });
  if (!workflow || workflow.workspaceId !== workspaceId) return null;
  return {
    ref: { kind: "workflow", id: workflow.id },
    label: normalizeCopilotItemId(workflow.name),
    workflowId: workflow.id,
  };
}

/**
 * The page the manager is on, derived from the route kind and the store and
 * never from typed text. `null` until the label resolves, so nothing is sent
 * with a message typed in the meantime.
 */
export function usePageContext(route: WorkspacePageRoute, workspaceId: string): PageContext | null {
  const task = useTaskContext(route.kind === "taskDetail" ? route.taskId : null, workspaceId);
  const workflow = useWorkflowContext(route.kind === "kanban", workspaceId);
  if (route.kind === "taskDetail") return task;
  return route.kind === "kanban" ? workflow : null;
}
