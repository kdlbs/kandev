import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { ReorderBand, ReorderedTaskPosition } from "@/lib/types/http";
import { mergeTaskRepositoryFields } from "@/lib/ws/handlers/task-repositories";
import { partitionWipTasks } from "@/lib/kanban/wip-queue";

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
  priority?: KanbanTask["priority"];
};

/**
 * Fall back to the multi-snapshot's own value only when the main kanban
 * lookup returned `undefined` (task absent from kanban.tasks). An explicit
 * `null` means the primary was intentionally cleared and must NOT be
 * replaced by a stale snapshot value.
 */
function preserveIfUndefined<T>(value: T | undefined, fallback: T | undefined): T | undefined {
  return value === undefined ? fallback : value;
}

function preserveMultiSnapshotFields(
  t: KanbanTask,
  fallback: KanbanTask | undefined,
): Pick<
  KanbanTask,
  | "primarySessionId"
  | "primarySessionState"
  | "primarySessionPendingAction"
  | "taskPendingAction"
  | "foregroundActivity"
  | "interrupted"
  | "autoStartFailed"
  | "workspaceOrphaned"
  | "priority"
> {
  return {
    primarySessionId: preserveIfUndefined(t.primarySessionId, fallback?.primarySessionId),
    primarySessionState: preserveIfUndefined(t.primarySessionState, fallback?.primarySessionState),
    primarySessionPendingAction: preserveIfUndefined(
      t.primarySessionPendingAction,
      fallback?.primarySessionPendingAction,
    ),
    taskPendingAction: preserveIfUndefined(t.taskPendingAction, fallback?.taskPendingAction),
    foregroundActivity: preserveIfUndefined(t.foregroundActivity, fallback?.foregroundActivity),
    interrupted: preserveIfUndefined(t.interrupted, fallback?.interrupted),
    autoStartFailed: preserveIfUndefined(t.autoStartFailed, fallback?.autoStartFailed),
    workspaceOrphaned: preserveIfUndefined(t.workspaceOrphaned, fallback?.workspaceOrphaned),
    priority: preserveIfUndefined(t.priority, fallback?.priority),
  };
}

function applyPositionsToTasks<T extends { id: string; position: number }>(
  tasks: T[],
  positionById: Map<string, number>,
): T[] {
  let changed = false;
  const next = tasks.map((task) => {
    const position = positionById.get(task.id);
    if (position === undefined || task.position === position) return task;
    changed = true;
    return { ...task, position };
  });
  return changed ? next : tasks;
}

const REORDER_BANDS: readonly ReorderBand[] = ["admitted", "queued"];

/**
 * Classifies every task id in an event's whole-step payload into the band it
 * currently belongs to, using this client's own last-known membership flags
 * (a reorder never changes band membership, so a stale-but-recent local copy
 * is a safe classifier). A task not found locally (e.g. not yet hydrated on
 * this client) is left unclassified so its position is applied rather than
 * held against nothing.
 */
function classifyTasksByBand(
  tasks: ReorderedTaskPosition[],
  stepId: string,
  membershipSources: KanbanTask[][],
): Map<string, ReorderBand> {
  const byId = new Map<string, ReorderBand>();
  for (const source of membershipSources) {
    const stepTasks = source.filter((task) => task.workflowStepId === stepId);
    if (stepTasks.length === 0) continue;
    const { admitted, queued } = partitionWipTasks(stepTasks, stepId);
    for (const task of admitted) if (!byId.has(task.id)) byId.set(task.id, "admitted");
    for (const task of queued) if (!byId.has(task.id)) byId.set(task.id, "queued");
  }
  const result = new Map<string, ReorderBand>();
  for (const task of tasks) {
    const band = byId.get(task.id);
    if (band) result.set(task.id, band);
  }
  return result;
}

function makeTaskReorderedHandler(store: StoreApi<AppState>): WsHandlers["task.reordered"] {
  return (message) => {
    const { workflow_step_id: stepId, revision, tasks } = message.payload;

    store.setState((state) => {
      // Asymmetric revision gate (Decision 11 / F31): an unsolicited event
      // only applies on a strictly-greater revision. A step with no recorded
      // revision (-1) accepts the first order it ever receives.
      const currentRevision = state.kanbanMulti.orderRevisionByStepId[stepId] ?? -1;
      if (revision <= currentRevision) {
        return state;
      }

      // AC.27: a band with a reorder request in flight keeps its optimistic
      // order until that request resolves; this whole-step payload's
      // position for such a task is held rather than applied, while every
      // other task in the same payload (the sibling band, or another step
      // entirely) is applied immediately.
      const pendingBands = REORDER_BANDS.filter(
        (band) => state.kanbanMulti.pendingReorderBandKeys[`${stepId}:${band}`],
      );
      const bandByTaskId = pendingBands.length
        ? classifyTasksByBand(tasks, stepId, [
            state.kanban.tasks,
            ...Object.values(state.kanbanMulti.snapshots).map((s) => s.tasks),
          ])
        : new Map<string, ReorderBand>();

      const held: Record<ReorderBand, ReorderedTaskPosition[]> = { admitted: [], queued: [] };
      const applyNow: ReorderedTaskPosition[] = [];
      for (const task of tasks) {
        const band = bandByTaskId.get(task.id);
        if (band && pendingBands.includes(band)) {
          held[band].push(task);
        } else {
          applyNow.push(task);
        }
      }

      let nextState = state;
      for (const band of pendingBands) {
        if (held[band].length === 0) continue;
        nextState = {
          ...nextState,
          kanbanMulti: {
            ...nextState.kanbanMulti,
            withheldReorderByBandKey: {
              ...nextState.kanbanMulti.withheldReorderByBandKey,
              [`${stepId}:${band}`]: { revision, tasks: held[band] },
            },
          },
        };
      }

      if (applyNow.length === 0) {
        // Every task in this payload belongs to a band still in flight —
        // nothing observable changes yet, so the revision scalar is left
        // alone; the withheld snapshot above carries the revision forward
        // for reconciliation once the in-flight request resolves.
        return nextState;
      }

      const positionById = new Map(applyNow.map((task) => [task.id, task.position]));
      const nextKanbanTasks = applyPositionsToTasks(nextState.kanban.tasks, positionById);
      let snapshotsChanged = false;
      const nextSnapshots: typeof nextState.kanbanMulti.snapshots = {};
      for (const [workflowId, snapshot] of Object.entries(nextState.kanbanMulti.snapshots)) {
        const nextTasks = applyPositionsToTasks(snapshot.tasks, positionById);
        if (nextTasks !== snapshot.tasks) {
          snapshotsChanged = true;
          nextSnapshots[workflowId] = { ...snapshot, tasks: nextTasks };
        } else {
          nextSnapshots[workflowId] = snapshot;
        }
      }

      return {
        ...nextState,
        kanban:
          nextKanbanTasks === nextState.kanban.tasks
            ? nextState.kanban
            : { ...nextState.kanban, tasks: nextKanbanTasks },
        kanbanMulti: {
          ...nextState.kanbanMulti,
          snapshots: snapshotsChanged ? nextSnapshots : nextState.kanbanMulti.snapshots,
          orderRevisionByStepId: {
            ...nextState.kanbanMulti.orderRevisionByStepId,
            [stepId]: revision,
          },
        },
      };
    });
  };
}

export function registerKanbanHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.reordered": makeTaskReorderedHandler(store),
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
        auto_advance_requires_signal: step.auto_advance_requires_signal,
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
              workspaceOrphaned: existing?.workspaceOrphaned,
              foregroundActivity: existing?.foregroundActivity,
              priority: preserveIfUndefined(task.priority, existing?.priority),
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
            return {
              ...t,
              ...repoFields,
              ...preserveMultiSnapshotFields(t, fallback),
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
