import { toKanbanTask } from "@/lib/kanban/map-task";
import type { WorkflowSnapshot } from "@/lib/types/http";

// Map workflow snapshot to kanban state on workspace switch.
export function mapSnapshotToKanban(snapshot: WorkflowSnapshot, newWorkflowId: string) {
  return {
    workflowId: newWorkflowId,
    isLoading: false,
    steps: snapshot.steps.map((step) => ({
      id: step.id,
      title: step.name,
      color: step.color,
      position: step.position,
      events: step.events,
      // Preserve optional step capabilities until the next full reload.
      allow_manual_move: step.allow_manual_move,
      prompt: step.prompt,
      is_start_step: step.is_start_step,
      show_in_command_panel: step.show_in_command_panel,
      agent_profile_id: step.agent_profile_id,
    })),
    tasks: snapshot.tasks.map(toKanbanTask),
  };
}

export function sortByUpdatedAtDesc<T extends { updated_at?: string | null }>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    const aDate = a.updated_at ? new Date(a.updated_at).getTime() : 0;
    const bDate = b.updated_at ? new Date(b.updated_at).getTime() : 0;
    return bDate - aDate;
  });
}
