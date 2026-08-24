import type { AppState, KanbanState } from "@/lib/state/store";
import { primaryTaskRepository } from "@/lib/types/http";
import type { WorkflowSnapshot, Message, Task } from "@/lib/types/http";
import { pickPendingAction, workspaceModeFromMetadata } from "@/lib/kanban/map-task";
import {
  isPRReviewFromMetadata,
  isIssueWatchFromMetadata,
  issueFieldsFromMetadata,
} from "@/lib/metadata-utils";

type KanbanTask = KanbanState["tasks"][number];

// Split out so the snapshot->task mapper (already at the complexity limit
// from its long list of `??` fallbacks) doesn't need to absorb one more.
function resolveAutoStartFailed(task: WorkflowSnapshot["tasks"][number]): boolean {
  return task.auto_start_failed ?? false;
}

function primaryExecutorFields(task: Task) {
  return {
    primaryExecutorId: task.primary_executor_id ?? undefined,
    primaryExecutorType: task.primary_executor_type ?? undefined,
    primaryExecutorName: task.primary_executor_name ?? undefined,
    isRemoteExecutor: task.is_remote_executor ?? false,
  };
}

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
        workflowId: snapshot.workflow.id,
        workflowStepId,
        title: task.title,
        description: task.description ?? undefined,
        autopilot: task.autopilot ?? false,
        position: task.position ?? 0,
        state: task.state,
        // Preserve WIP admission and queue metadata during HTTP snapshot
        // refreshes. The Go boot mapper, WebSocket handler, and canonical
        // task mapper already carry these fields. This path must keep the
        // same values so queue classification and ordering stay consistent
        // after a workflow switch or reconnect.
        priority: task.priority,
        createdAt: task.created_at,
        wipAdmitted: task.wip_admitted,
        queuedForStepId: task.queued_for_step_id,
        queuedAt: task.queued_at,
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
        ...primaryExecutorFields(task),
        primarySessionPendingAction: pickPendingAction(task.primary_session_pending_action),
        taskPendingAction: pickPendingAction(task.task_pending_action),
        foregroundActivity: task.foreground_activity ?? undefined,
        autoStartFailed: resolveAutoStartFailed(task),
        activeSubagentCount: task.active_subagent_count ?? undefined,
        sessionCount: task.session_count ?? undefined,
        reviewStatus: task.review_status ?? undefined,
        statusSummary: task.status_summary,
        parentTaskId: task.parent_id ?? undefined,
        metadata: task.metadata,
        workspaceMode: workspaceModeFromMetadata(task.metadata),
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
        wip_limit: step.wip_limit,
        pull_from_step_id: step.pull_from_step_id ?? null,
        stage_type: step.stage_type,
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
      lastSessionByTaskId: resolvedSessionId ? { [task.id]: resolvedSessionId } : {},
      resumeSkippedSessionIds: {},
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
                isLoadingMore: false,
                hasMore: messages.hasMore ?? false,
                oldestCursor: messages.oldestCursor ?? messages.items[0]?.id ?? null,
              },
            },
          }
        : undefined,
  };
}
