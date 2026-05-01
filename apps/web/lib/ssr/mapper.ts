import type { AppState, KanbanState } from "@/lib/state/store";
import { primaryTaskRepository } from "@/lib/types/http";
import type { WorkflowSnapshot, Message, Task } from "@/lib/types/http";
import {
  isPRReviewFromMetadata,
  isIssueWatchFromMetadata,
  issueFieldsFromMetadata,
} from "@/lib/metadata-utils";

type KanbanTask = KanbanState["tasks"][number];

export function snapshotToState(snapshot: WorkflowSnapshot): Partial<AppState> {
  // Handle empty snapshot (ephemeral tasks have no workflow)
  if (!snapshot.workflow) {
    return {
      kanban: {
        workflowId: "",
        isLoading: false,
        steps: [],
        tasks: [],
      },
    };
  }

  const tasks = snapshot.tasks
    .filter((task) => !task.is_ephemeral) // Filter out ephemeral tasks (e.g., quick chat)
    .map((task) => {
      const workflowStepId = task.workflow_step_id;
      if (!workflowStepId) return null;
      const primary = primaryTaskRepository(task.repositories);
      return {
        id: task.id,
        workflowStepId,
        title: task.title,
        description: task.description ?? undefined,
        position: task.position ?? 0,
        state: task.state,
        repositoryId: primary?.repository_id ?? undefined,
        repositories: task.repositories?.map((r) => ({
          id: r.id,
          repository_id: r.repository_id,
          base_branch: r.base_branch,
          checkout_branch: r.checkout_branch,
          position: r.position,
        })),
        primarySessionId: task.primary_session_id ?? undefined,
        primarySessionState: task.primary_session_state ?? undefined,
        sessionCount: task.session_count ?? undefined,
        reviewStatus: task.review_status ?? undefined,
        parentTaskId: task.parent_id ?? undefined,
        updatedAt: task.updated_at,
        isPRReview: isPRReviewFromMetadata(task.metadata),
        isIssueWatch: isIssueWatchFromMetadata(task.metadata),
        ...issueFieldsFromMetadata(task.metadata),
      } as KanbanTask;
    })
    .filter((task): task is KanbanTask => task !== null);

  return {
    kanban: {
      workflowId: snapshot.workflow.id,
      isLoading: false,
      steps: snapshot.steps.map((step) => ({
        id: step.id,
        title: step.name,
        color: step.color ?? "bg-neutral-400",
        position: step.position,
        events: step.events,
        allow_manual_move: step.allow_manual_move,
        prompt: step.prompt,
        is_start_step: step.is_start_step,
        show_in_command_panel: step.show_in_command_panel,
        agent_profile_id: step.agent_profile_id,
      })),
      tasks,
    },
  };
}

export function taskToState(
  task: Task,
  sessionId?: string | null,
  messages?: { items: Message[]; hasMore?: boolean; oldestCursor?: string | null },
): Partial<AppState> {
  const resolvedSessionId = sessionId ?? messages?.items[0]?.session_id ?? null;
  return {
    tasks: {
      activeTaskId: task.id,
      activeSessionId: resolvedSessionId,
      pinnedSessionId: null,
    },
    messages:
      resolvedSessionId && messages
        ? {
            bySession: {
              [resolvedSessionId]: messages.items,
            },
            metaBySession: {
              [resolvedSessionId]: {
                isLoading: false,
                hasMore: messages.hasMore ?? false,
                oldestCursor: messages.oldestCursor ?? messages.items[0]?.id ?? null,
              },
            },
          }
        : undefined,
  };
}
