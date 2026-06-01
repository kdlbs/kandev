/**
 * Kanban domain WS → TanStack Query bridge.
 *
 * The sole writer for kanban / task / workflow WS events (the former Zustand
 * handlers — lib/ws/handlers/{kanban,tasks,workflows}.ts — were deleted once
 * every reader moved to TanStack Query).
 *
 * Single source of truth: `qk.kanban.multi()` holds the KanbanMultiData record
 * (per-workflow task/step snapshots); `qk.kanban.workflowsList(wsId)` holds the
 * workspace workflow list. All events patch those cache entries. Single-workflow
 * views are derived via `select` in consumers — no double writes.
 *
 * Timestamp conflict resolution: `applyIfNewer(prev, next)` compares
 * `updatedAt` before replacing a cached task so that a stale WS echo
 * cannot overwrite a fresher optimistic update.
 */
import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { TaskLike } from "@/lib/kanban/map-task";
import { qk } from "@/lib/query/keys";
import type { KanbanMultiData, WorkflowsListData } from "@/lib/query/query-options/kanban";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { wrapBridgeHandler } from "./index";

type KanbanTask = KanbanState["tasks"][number];
type KanbanStep = KanbanState["steps"][number];
type Snapshot = import("@/lib/state/slices/kanban/types").WorkflowSnapshotData;

// ---------------------------------------------------------------------------
// Timestamp conflict resolution
// ---------------------------------------------------------------------------

/**
 * Returns `next` only when it is strictly newer than `prev` (or when there is
 * no `prev` to compare against).  Falls back to `prev` when:
 *   - both have an `updatedAt` and `prev` is >= `next`, or
 *   - `next` has no `updatedAt` but `prev` does (prev timestamp is preferred).
 *
 * Exported so bridge tests can assert the merge logic independently.
 */
export function applyIfNewer(prev: KanbanTask | undefined, next: KanbanTask): KanbanTask {
  if (!prev) return next;
  const prevTs = prev.updatedAt;
  const nextTs = next.updatedAt;
  if ((!nextTs && prevTs) || (nextTs && prevTs && nextTs <= prevTs)) {
    // `next` is stale by timestamp, so the bulk of its fields (title,
    // position) must not clobber the fresher cached task. But some fields are
    // authoritative re-derivations the backend broadcasts on a `task.updated`
    // that does NOT bump the task row's `updated_at`:
    //   - primary-session fields: `SetSessionPrimary` only touches
    //     task_sessions, so the star would otherwise never move.
    //   - workflow step + task state: a workflow transition (e.g. In Progress
    //     → Review on turn-complete) can land in the SAME wall-clock second as
    //     the prior cached `updated_at`, so a `<=` tie reads as "stale" and the
    //     step change would be silently dropped (the stepper stays on the old
    //     step). These are server-authoritative, so fold them through too.
    // Everything else (title, position) is preserved from the kept task.
    return mergeAuthoritativeFields(prev, next);
  }
  return next;
}

/**
 * Returns `prev` with the server-authoritative fields (primary-session +
 * workflow step + task state) replaced by `next`'s values, but only when they
 * actually differ. Used by {@link applyIfNewer} to let a timestamp-stale (or
 * same-second) `task.updated` still carry these through: the backend
 * re-derives them from the DB and may broadcast without bumping the task row's
 * `updated_at` (`session.set_primary`) or within the same wall-clock second as
 * the prior update (workflow step transition on turn-complete). Title/position
 * are NOT folded — a genuinely stale echo must not clobber those.
 */
function mergeAuthoritativeFields(prev: KanbanTask, next: KanbanTask): KanbanTask {
  let merged = prev;

  // Workflow step + task state: a defined, different value is authoritative.
  // (`task.updated` always carries the current step; `toKanbanTask` maps a
  // missing step to "" so guard against that clearing a real step.)
  if (next.workflowStepId && next.workflowStepId !== prev.workflowStepId) {
    merged = { ...merged, workflowStepId: next.workflowStepId };
  }
  if (next.state !== undefined && next.state !== prev.state) {
    merged = { ...merged, state: next.state };
  }

  // Primary-session fields. A stale event that simply OMITS the primary id
  // (undefined) carries no information — never let it clear a known primary.
  // Only a defined value (the new primary id, or an explicit null clear) and a
  // genuine difference is applied.
  if (
    next.primarySessionId !== undefined &&
    !(
      next.primarySessionId === prev.primarySessionId &&
      next.primarySessionState === prev.primarySessionState
    )
  ) {
    merged = {
      ...merged,
      primarySessionId: next.primarySessionId,
      primarySessionState: next.primarySessionState,
    };
  }

  return merged;
}

// ---------------------------------------------------------------------------
// Snapshot mutators (pure — no side effects, used inside setQueryData)
// ---------------------------------------------------------------------------

function stepFromWsPayload(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  step: any,
  index: number,
): KanbanStep {
  return {
    id: step.id as string,
    title: (step.name ?? step.title) as string,
    color: (step.color ?? "bg-neutral-400") as string,
    position: (step.position ?? index) as number,
    events: step.events,
    show_in_command_panel: step.show_in_command_panel,
    allow_manual_move: step.allow_manual_move,
    prompt: step.prompt,
    is_start_step: step.is_start_step,
    agent_profile_id: step.agent_profile_id,
    stage_type: step.stage_type,
  };
}

/**
 * Builds tasks from a `kanban.update` payload, preserving primarySessionId /
 * primarySessionState from the cache when the event omits them (backend uses
 * omitempty). Preserves null sentinel values (intentional clears).
 */
function buildTasksFromKanbanUpdate(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payloadTasks: any[],
  existingById: Map<string, KanbanTask>,
): KanbanTask[] {
  return payloadTasks
    .filter((t) => !t.is_ephemeral)
    .map((t) => {
      const existing = existingById.get(t.id as string);
      return {
        id: t.id as string,
        workflowStepId: t.workflowStepId as string,
        title: t.title as string,
        description: t.description as string | undefined,
        position: (t.position ?? 0) as number,
        state: t.state,
        // Preserve primarySessionId from cache when event omits it (undefined),
        // but NOT when the event explicitly sets it to null (intentional clear).
        primarySessionId:
          t.primarySessionId === undefined ? existing?.primarySessionId : t.primarySessionId,
        primarySessionState:
          t.primarySessionState === undefined
            ? existing?.primarySessionState
            : t.primarySessionState,
      } as KanbanTask;
    });
}

function upsertTaskInSnapshot(snapshot: Snapshot, nextTask: KanbanTask): Snapshot {
  const prevById = new Map(snapshot.tasks.map((t) => [t.id, t]));
  const merged = applyIfNewer(prevById.get(nextTask.id), nextTask);
  const exists = prevById.has(nextTask.id);
  const tasks = exists
    ? snapshot.tasks.map((t) => (t.id === nextTask.id ? merged : t))
    : [...snapshot.tasks, merged];
  return { ...snapshot, tasks };
}

function removeTaskFromSnapshot(snapshot: Snapshot, taskId: string): Snapshot {
  return { ...snapshot, tasks: snapshot.tasks.filter((t) => t.id !== taskId) };
}

// ---------------------------------------------------------------------------
// Cache updater helpers
// ---------------------------------------------------------------------------

type Updater = (prev: KanbanMultiData | undefined) => KanbanMultiData | undefined;

function patchSnapshot(wfId: string, fn: (snap: Snapshot) => Snapshot): Updater {
  return (prev) => {
    if (!prev) return prev;
    const snap = prev.snapshots[wfId];
    if (!snap) return prev;
    return {
      ...prev,
      snapshots: { ...prev.snapshots, [wfId]: fn(snap) },
    };
  };
}

function setSnapshot(wfId: string, snap: Snapshot): Updater {
  return (prev) => {
    if (!prev) return { snapshots: { [wfId]: snap } };
    return { ...prev, snapshots: { ...prev.snapshots, [wfId]: snap } };
  };
}

// ---------------------------------------------------------------------------
// Bridge sub-registrars (each ≤ 50 lines, grouped by event family)
// ---------------------------------------------------------------------------

function registerKanbanUpdateHandler(
  ws: WebSocketClient,
  queryClient: QueryClient,
  multi: ReturnType<typeof qk.kanban.multi>,
): () => void {
  return ws.on(
    "kanban.update",
    wrapBridgeHandler(queryClient, "kanban.update", (message) => {
      const {
        workflowId,
        steps: rawSteps,
        tasks: rawTasks,
      } = message.payload as {
        workflowId: string;
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        steps: any[];
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        tasks: any[];
      };
      const steps = rawSteps.map((s, idx) => stepFromWsPayload(s, idx));
      queryClient.setQueryData<KanbanMultiData>(multi, (prev) => {
        const existingTasks = prev?.snapshots[workflowId]?.tasks ?? [];
        const existingById = new Map(existingTasks.map((t) => [t.id, t]));
        const tasks = buildTasksFromKanbanUpdate(rawTasks, existingById);
        const snap: Snapshot = {
          workflowId,
          workflowName: prev?.snapshots[workflowId]?.workflowName ?? workflowId,
          steps,
          tasks,
        };
        if (!prev) return { snapshots: { [workflowId]: snap } };
        return { ...prev, snapshots: { ...prev.snapshots, [workflowId]: snap } };
      });
    }),
  );
}

function registerTaskHandlers(
  ws: WebSocketClient,
  queryClient: QueryClient,
  multi: ReturnType<typeof qk.kanban.multi>,
): Array<() => void> {
  const unsubCreated = ws.on(
    "task.created",
    wrapBridgeHandler(queryClient, "task.created", (message) => {
      const payload = message.payload as TaskLike & {
        workflow_id: string;
        is_ephemeral?: boolean;
      };
      if (payload.is_ephemeral) return;
      const task = toKanbanTask(payload);
      queryClient.setQueryData<KanbanMultiData>(
        multi,
        patchSnapshot(payload.workflow_id, (snap) => upsertTaskInSnapshot(snap, task)),
      );
    }),
  );

  const unsubStateChanged = ws.on(
    "task.state_changed",
    wrapBridgeHandler(queryClient, "task.state_changed", (message) => {
      const payload = message.payload as TaskLike & {
        workflow_id: string;
        is_ephemeral?: boolean;
      };
      if (payload.is_ephemeral) return;
      const task = toKanbanTask(payload);
      queryClient.setQueryData<KanbanMultiData>(
        multi,
        patchSnapshot(payload.workflow_id, (snap) => upsertTaskInSnapshot(snap, task)),
      );
    }),
  );

  const unsubDeleted = ws.on(
    "task.deleted",
    wrapBridgeHandler(queryClient, "task.deleted", (message) => {
      const { task_id: taskId, workflow_id: wfId } = message.payload as {
        task_id: string;
        workflow_id: string;
      };
      queryClient.setQueryData<KanbanMultiData>(
        multi,
        patchSnapshot(wfId, (snap) => removeTaskFromSnapshot(snap, taskId)),
      );
    }),
  );

  const unsubUpdated = ws.on(
    "task.updated",
    wrapBridgeHandler(queryClient, "task.updated", (message) => {
      const payload = message.payload as TaskLike & {
        workflow_id: string;
        old_workflow_id?: string | null;
        is_ephemeral?: boolean;
        archived_at?: string | null;
        task_id: string;
      };
      if (payload.is_ephemeral) return;
      queryClient.setQueryData<KanbanMultiData>(multi, applyTaskUpdated(payload));
    }),
  );

  return [unsubCreated, unsubStateChanged, unsubDeleted, unsubUpdated];
}

type TaskUpdatedPayload = TaskLike & {
  workflow_id: string;
  old_workflow_id?: string | null;
  archived_at?: string | null;
  task_id: string;
};

function applyTaskUpdated(payload: TaskUpdatedPayload): Updater {
  return (prev) => {
    if (!prev) return prev;
    const { workflow_id: wfId, old_workflow_id: oldWfId, task_id: taskId } = payload;
    let next = prev;
    if (oldWfId && oldWfId !== wfId) {
      const oldSnap = next.snapshots[oldWfId];
      if (oldSnap) {
        next = {
          ...next,
          snapshots: { ...next.snapshots, [oldWfId]: removeTaskFromSnapshot(oldSnap, taskId) },
        };
      }
    }
    if (payload.archived_at) {
      const snap = next.snapshots[wfId];
      if (snap) {
        next = {
          ...next,
          snapshots: { ...next.snapshots, [wfId]: removeTaskFromSnapshot(snap, taskId) },
        };
      }
      return next;
    }
    const snap = next.snapshots[wfId];
    if (!snap) return next;
    const task = toKanbanTask(payload);
    return { ...next, snapshots: { ...next.snapshots, [wfId]: upsertTaskInSnapshot(snap, task) } };
  };
}

function registerWorkflowHandlers(
  ws: WebSocketClient,
  queryClient: QueryClient,
  multi: ReturnType<typeof qk.kanban.multi>,
): Array<() => void> {
  const unsubUpdated = ws.on(
    "workflow.updated",
    wrapBridgeHandler(queryClient, "workflow.updated", (message) => {
      const payload = message.payload as { id: string; name: string; hidden?: boolean };
      queryClient.setQueryData<KanbanMultiData>(multi, (prev) => {
        if (!prev) return prev;
        const snap = prev.snapshots[payload.id];
        if (!snap) return prev;
        return setSnapshot(payload.id, { ...snap, workflowName: payload.name })(prev);
      });
    }),
  );

  const unsubDeleted = ws.on(
    "workflow.deleted",
    wrapBridgeHandler(queryClient, "workflow.deleted", (message) => {
      const { id: wfId } = message.payload as { id: string };
      queryClient.setQueryData<KanbanMultiData>(multi, (prev) => {
        if (!prev) return prev;
        const { [wfId]: _removed, ...rest } = prev.snapshots;
        return { ...prev, snapshots: rest };
      });
    }),
  );

  const unsubStepCreated = ws.on(
    "workflow.step.created",
    wrapBridgeHandler(queryClient, "workflow.step.created", (message) => {
      const step = (message.payload as { step: { workflow_id: string } & Record<string, unknown> })
        .step;
      const wfId = step.workflow_id as string;
      queryClient.setQueryData<KanbanMultiData>(
        multi,
        patchSnapshot(wfId, (snap) => {
          if (snap.steps.some((s) => s.id === step.id)) return snap;
          const steps = [...snap.steps, stepFromWsPayload(step, snap.steps.length)].sort(
            (a, b) => a.position - b.position,
          );
          return { ...snap, steps };
        }),
      );
    }),
  );

  const unsubStepUpdated = ws.on(
    "workflow.step.updated",
    wrapBridgeHandler(queryClient, "workflow.step.updated", (message) => {
      const step = (message.payload as { step: { workflow_id: string } & Record<string, unknown> })
        .step;
      const wfId = step.workflow_id as string;
      queryClient.setQueryData<KanbanMultiData>(
        multi,
        patchSnapshot(wfId, (snap) => {
          const idx = snap.steps.findIndex((s) => s.id === step.id);
          const steps =
            idx >= 0
              ? snap.steps
                  .map((s) => (s.id === step.id ? stepFromWsPayload(step, idx) : s))
                  .sort((a, b) => a.position - b.position)
              : snap.steps;
          return { ...snap, steps };
        }),
      );
    }),
  );

  const unsubStepDeleted = ws.on(
    "workflow.step.deleted",
    wrapBridgeHandler(queryClient, "workflow.step.deleted", (message) => {
      const step = (message.payload as { step: { workflow_id: string; id: string } }).step;
      queryClient.setQueryData<KanbanMultiData>(
        multi,
        patchSnapshot(step.workflow_id, (snap) => ({
          ...snap,
          steps: snap.steps.filter((s) => s.id !== step.id),
        })),
      );
    }),
  );

  return [unsubUpdated, unsubDeleted, unsubStepCreated, unsubStepUpdated, unsubStepDeleted];
}

// ---------------------------------------------------------------------------
// Workflows-list handlers (the `qk.kanban.workflowsList(wsId)` cache)
// ---------------------------------------------------------------------------

type WorkflowItem = WorkflowsListData[number];

function workflowItemFromPayload(payload: {
  id: string;
  workspace_id: string;
  name: string;
  description?: string;
  agent_profile_id?: string;
  hidden?: boolean;
  style?: WorkflowItem["style"];
}): WorkflowItem {
  return {
    id: payload.id,
    workspaceId: payload.workspace_id,
    name: payload.name,
    description: payload.description,
    agent_profile_id: payload.agent_profile_id,
    hidden: payload.hidden,
    style: payload.style,
  };
}

/**
 * Applies a mutation to the workspace-scoped workflows-list cache entry.
 * The list is keyed by workspaceId, so we scope the update to the entry for
 * `workspaceId` (matched against the `["kanban","workflows-list",wsId]` key).
 */
function setWorkflowsList(
  queryClient: QueryClient,
  workspaceId: string,
  fn: (prev: WorkflowsListData) => WorkflowsListData,
): void {
  queryClient.setQueryData<WorkflowsListData>(qk.kanban.workflowsList(workspaceId), (prev) =>
    prev ? fn(prev) : prev,
  );
}

function registerWorkflowsListHandlers(
  ws: WebSocketClient,
  queryClient: QueryClient,
): Array<() => void> {
  const unsubCreated = ws.on(
    "workflow.created",
    wrapBridgeHandler(queryClient, "workflow.created", (message) => {
      const payload = message.payload as Parameters<typeof workflowItemFromPayload>[0];
      setWorkflowsList(queryClient, payload.workspace_id, (prev) =>
        prev.some((w) => w.id === payload.id) ? prev : [workflowItemFromPayload(payload), ...prev],
      );
    }),
  );

  const unsubUpdated = ws.on(
    "workflow.updated",
    wrapBridgeHandler(queryClient, "workflow.list.updated", (message) => {
      const payload = message.payload as Parameters<typeof workflowItemFromPayload>[0];
      setWorkflowsList(queryClient, payload.workspace_id, (prev) =>
        prev.map((w) =>
          w.id === payload.id
            ? {
                ...w,
                name: payload.name,
                agent_profile_id: payload.agent_profile_id,
                hidden: payload.hidden !== undefined ? Boolean(payload.hidden) : w.hidden,
                style: payload.style ?? w.style,
              }
            : w,
        ),
      );
    }),
  );

  const unsubDeleted = ws.on(
    "workflow.deleted",
    wrapBridgeHandler(queryClient, "workflow.list.deleted", (message) => {
      const payload = message.payload as { id: string; workspace_id?: string };
      if (payload.workspace_id) {
        setWorkflowsList(queryClient, payload.workspace_id, (prev) =>
          prev.filter((w) => w.id !== payload.id),
        );
        return;
      }
      // Older payloads may omit workspace_id — sweep every list cache entry.
      queryClient.setQueriesData<WorkflowsListData>(
        { queryKey: ["kanban", "workflows-list"] },
        (prev) => (prev ? prev.filter((w) => w.id !== payload.id) : prev),
      );
    }),
  );

  return [unsubCreated, unsubUpdated, unsubDeleted];
}

// ---------------------------------------------------------------------------
// Bridge registrar
// ---------------------------------------------------------------------------

/**
 * Registers WS handlers for kanban, task, and workflow events.
 * Returns a cleanup function that unsubscribes all handlers.
 */
export function registerKanbanBridge(ws: WebSocketClient, queryClient: QueryClient): () => void {
  const multi = qk.kanban.multi();
  const unsubKanbanUpdate = registerKanbanUpdateHandler(ws, queryClient, multi);
  const taskUnsubs = registerTaskHandlers(ws, queryClient, multi);
  const workflowUnsubs = registerWorkflowHandlers(ws, queryClient, multi);
  const workflowsListUnsubs = registerWorkflowsListHandlers(ws, queryClient);
  const allUnsubs = [unsubKanbanUpdate, ...taskUnsubs, ...workflowUnsubs, ...workflowsListUnsubs];
  return () => {
    for (const unsub of allUnsubs) unsub();
  };
}
