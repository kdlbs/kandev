import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { mergeTaskRepositoryFields } from "@/lib/ws/handlers/task-repositories";

type KanbanTask = KanbanState["tasks"][number];
type KanbanStep = KanbanState["steps"][number];

function queueFields(
  task: KanbanUpdateTask,
  existing: KanbanTask | undefined,
): Pick<KanbanTask, "wipAdmitted" | "queuedForStepId" | "queuedAt"> {
  return {
    wipAdmitted: task.wip_admitted ?? task.wipAdmitted ?? existing?.wipAdmitted,
    queuedForStepId: task.queued_for_step_id ?? task.queuedForStepId ?? existing?.queuedForStepId,
    queuedAt: task.queued_at ?? task.queuedAt ?? existing?.queuedAt,
  };
}

/**
 * Dependency fields are derived server-side and re-sent whole on every
 * task.updated, so an absent list means "no edges", not "unchanged" — falling
 * back to the existing value would leave a stale badge after the last edge is
 * removed. Only fall back when the payload omits the field entirely (a partial
 * update that never touched dependencies).
 */
const DEPENDENCY_PAYLOAD_KEYS = [
  "blocked",
  "blocked_reason",
  "blockedReason",
  "depends_on",
  "dependsOn",
  "blocks",
  "start_when_unblocked",
  "startWhenUnblocked",
] as const;

function touchesDependencies(task: KanbanUpdateTask): boolean {
  return DEPENDENCY_PAYLOAD_KEYS.some((key) => key in task);
}

function dependencyFields(
  task: KanbanUpdateTask,
  existing: KanbanTask | undefined,
): Pick<KanbanTask, "blocked" | "blockedReason" | "dependsOn" | "blocks" | "startWhenUnblocked"> {
  if (!touchesDependencies(task)) {
    return {
      blocked: existing?.blocked,
      blockedReason: existing?.blockedReason,
      dependsOn: existing?.dependsOn,
      blocks: existing?.blocks,
      startWhenUnblocked: existing?.startWhenUnblocked,
    };
  }
  return {
    blocked: task.blocked ?? false,
    blockedReason: task.blocked_reason ?? task.blockedReason,
    dependsOn: task.depends_on ?? task.dependsOn ?? [],
    blocks: task.blocks ?? [],
    startWhenUnblocked: task.start_when_unblocked ?? task.startWhenUnblocked ?? false,
  };
}

type KanbanUpdateTask = {
  id: string;
  workflowStepId: string;
  title: string;
  description?: string;
  position?: number;
  state?: KanbanTask["state"];
  repository_id?: string;
  repositories?: KanbanTask["repositories"];
  is_ephemeral?: boolean;
  wip_admitted?: boolean;
  queued_for_step_id?: string;
  queued_at?: string;
  wipAdmitted?: boolean;
  queuedForStepId?: string;
  queuedAt?: string;
  blocked?: boolean;
  blocked_reason?: string;
  blockedReason?: string;
  depends_on?: KanbanTask["dependsOn"];
  dependsOn?: KanbanTask["dependsOn"];
  blocks?: KanbanTask["blocks"];
  start_when_unblocked?: boolean;
  startWhenUnblocked?: boolean;
};

export function registerKanbanHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "kanban.update": (message) => {
      const workflowId = message.payload.workflowId;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const steps: KanbanStep[] = message.payload.steps.map((step: any, index: number) => ({
        id: step.id,
        title: step.title,
        color: step.color ?? "bg-neutral-400",
        position: step.position ?? index,
        events: step.events,
        show_in_command_panel: step.show_in_command_panel,
        agent_profile_id: step.agent_profile_id,
        wip_limit: step.wip_limit,
        pull_from_step_id: step.pull_from_step_id ?? null,
      }));

      store.setState((state) => {
        // kanban.update doesn't carry primarySessionId / primarySessionState —
        // those are set by task.updated WS events. Build tasks inside setState
        // so we can read existing values and preserve them.
        const existingById = new Map(state.kanban.tasks.map((t) => [t.id, t]));
        const tasks: KanbanTask[] = message.payload.tasks
          // Filter out ephemeral tasks (e.g., quick chat)
          .filter((task: KanbanUpdateTask) => !task.is_ephemeral)
          .map((task: KanbanUpdateTask) => {
            const existing = existingById.get(task.id);
            const repoFields = mergeTaskRepositoryFields(existing, {
              repositoryId: task.repository_id,
              repositories: task.repositories,
            });
            return {
              id: task.id,
              workflowId,
              workflowStepId: task.workflowStepId,
              title: task.title,
              description: task.description,
              position: task.position ?? 0,
              state: task.state,
              ...repoFields,
              primarySessionId: existing?.primarySessionId,
              primarySessionState: existing?.primarySessionState,
              primarySessionPendingAction: existing?.primarySessionPendingAction,
              taskPendingAction: existing?.taskPendingAction,
              interrupted: existing?.interrupted,
              autoStartFailed: existing?.autoStartFailed,
              foregroundActivity: existing?.foregroundActivity,
              ...queueFields(task, existing),
              ...dependencyFields(task, existing),
            };
          });

        const next = {
          ...state,
          kanban: { workflowId, steps, tasks },
        };

        // Also update multi-workflow snapshots if this workflow is tracked
        const snapshot = state.kanbanMulti.snapshots[workflowId];
        if (snapshot) {
          const existingMultiById = new Map(snapshot.tasks.map((t) => [t.id, t]));
          const multiTasks = tasks.map((t) => {
            const fallback = existingMultiById.get(t.id);
            const repoFields = mergeTaskRepositoryFields(fallback, t);
            // Fall back to the multi-snapshot's own value only when the main
            // kanban lookup returned `undefined` (task absent from kanban.tasks).
            // An explicit `null` means the primary was intentionally cleared
            // and must NOT be replaced by a stale snapshot value.
            return {
              ...t,
              ...repoFields,
              primarySessionId:
                t.primarySessionId === undefined ? fallback?.primarySessionId : t.primarySessionId,
              primarySessionState:
                t.primarySessionState === undefined
                  ? fallback?.primarySessionState
                  : t.primarySessionState,
              primarySessionPendingAction:
                t.primarySessionPendingAction === undefined
                  ? fallback?.primarySessionPendingAction
                  : t.primarySessionPendingAction,
              taskPendingAction:
                t.taskPendingAction === undefined
                  ? fallback?.taskPendingAction
                  : t.taskPendingAction,
              foregroundActivity:
                t.foregroundActivity === undefined
                  ? fallback?.foregroundActivity
                  : t.foregroundActivity,
              interrupted: t.interrupted === undefined ? fallback?.interrupted : t.interrupted,
              autoStartFailed:
                t.autoStartFailed === undefined ? fallback?.autoStartFailed : t.autoStartFailed,
            };
          });
          return {
            ...next,
            kanbanMulti: {
              ...next.kanbanMulti,
              snapshots: {
                ...next.kanbanMulti.snapshots,
                // A full kanban.update carries the authoritative step list;
                // clear any placeholder marker so final-step gating can
                // resolve against the real steps.
                [workflowId]: { ...snapshot, steps, tasks: multiTasks, isPlaceholder: false },
              },
            },
          };
        }

        return next;
      });
    },
  };
}
