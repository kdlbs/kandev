import type { StateCreator } from "zustand";
import type { AppState } from "../app-state-types";
import type { KanbanState, WorkflowSnapshotData } from "./kanban/types";
import {
  emptyTaskOverviewState,
  type TaskOverview,
  type TaskOverviewState,
} from "./task-overview-types";
import { mergeTaskOverview, recordTaskOverviewChange } from "./task-overview-merge";

type Creator = StateCreator<AppState, [], [["zustand/immer", never]]>;

/** All legacy board writes enter the same transaction as canonical overview writes. */
export function withTaskOverviewNormalization(create: Creator): Creator {
  return (set, get, api) => {
    const normalizedSet: typeof set = (update, replace) =>
      set((previous) => {
        const partial = typeof update === "function" ? update(previous) : update;
        if (partial === previous) return previous;
        const next = replace ? (partial as AppState) : { ...previous, ...partial };
        return normalizeTaskOverviews(previous, next);
      }, true);
    api.setState = normalizedSet;
    return normalizeTaskOverviews(undefined, create(normalizedSet, get, api));
  };
}

export function taskOverviewScope(state: AppState): string {
  return JSON.stringify([
    state.workspaces.activeId,
    state.workspaceContextGeneration,
    state.auth.mode,
    state.auth.authenticated,
    state.auth.user?.id,
  ]);
}

function overviewInputsChanged(previous: AppState | undefined, next: AppState): boolean {
  return (
    !previous ||
    previous.kanban !== next.kanban ||
    previous.kanbanMulti !== next.kanbanMulti ||
    previous.workflows !== next.workflows ||
    previous.sidebarArchivedTasks.itemsByWorkspaceId !==
      next.sidebarArchivedTasks.itemsByWorkspaceId ||
    previous.taskOverview.byId !== next.taskOverview.byId ||
    previous.taskOverview.owners !== next.taskOverview.owners ||
    previous.tasks.activeTaskId !== next.tasks.activeTaskId
  );
}

function retainProjection(
  state: TaskOverviewState,
  source: { owner: string; workflowId?: string | null },
  incoming: TaskOverview[],
  previous: TaskOverview[] | undefined,
) {
  if (incoming !== previous) {
    const before = new Map(previous?.map((task) => [task.id, task]));
    for (const task of incoming) {
      if (task !== before.get(task.id)) {
        const current = state.byId[task.id];
        const projection =
          task.workflowId === undefined && source.workflowId
            ? { ...task, workflowId: source.workflowId }
            : task;
        const merged = mergeTaskOverview(current, projection);
        if (current && merged !== current) {
          const patch = Object.fromEntries(
            Object.entries(merged).filter(
              ([key, value]) => !Object.is(value, current[key as keyof TaskOverview]),
            ),
          );
          Object.assign(state, recordTaskOverviewChange(state, task.id, { ...patch, id: task.id }));
        }
        state.byId[task.id] = merged;
      }
    }
    state.owners[source.owner] = incoming.map((task) => task.id);
  }
}

function collectBoardOwners(
  previous: AppState | undefined,
  next: AppState,
  overview: TaskOverviewState,
) {
  retainProjection(
    overview,
    { owner: "board", workflowId: next.kanban.workflowId },
    next.kanban.tasks,
    previous?.kanban.tasks,
  );
  for (const [id, snapshot] of Object.entries(next.kanbanMulti.snapshots)) {
    retainProjection(
      overview,
      { owner: `workflow:${id}`, workflowId: snapshot.workflowId ?? id },
      snapshot.tasks,
      previous?.kanbanMulti.snapshots[id]?.tasks,
    );
  }
  for (const [id, tasks] of Object.entries(next.sidebarArchivedTasks.itemsByWorkspaceId)) {
    retainProjection(
      overview,
      { owner: `archive:${id}` },
      tasks,
      previous?.sidebarArchivedTasks.itemsByWorkspaceId[id],
    );
  }
  for (const owner of Object.keys(overview.owners)) {
    if (owner.startsWith("workflow:") && !next.kanbanMulti.snapshots[owner.slice(9)])
      delete overview.owners[owner];
    if (
      owner.startsWith("archive:") &&
      !next.sidebarArchivedTasks.itemsByWorkspaceId[owner.slice(8)]
    )
      delete overview.owners[owner];
  }
  pruneBoardOwners(next, overview);
  const activeId = next.tasks.activeTaskId;
  overview.owners.detail = activeId && overview.byId[activeId] ? [activeId] : [];
}

function pruneBoardOwners(next: AppState, overview: TaskOverviewState) {
  for (const [owner, workflowId] of [
    ["board", next.kanban.workflowId],
    ...Object.keys(next.kanbanMulti.snapshots).map((id) => [`workflow:${id}`, id]),
  ] as Array<[string, string | null]>) {
    overview.owners[owner] = (overview.owners[owner] ?? []).filter((id) => {
      const task = overview.byId[id];
      return task && !task.isArchived && (!workflowId || task.workflowId === workflowId);
    });
  }
}

function releaseUnownedRecords(overview: TaskOverviewState) {
  const retained = new Set(Object.values(overview.owners).flat());
  for (const id of Object.keys(overview.byId)) {
    if (!retained.has(id)) delete overview.byId[id];
  }
}

function canonicalTasks(
  tasks: TaskOverview[],
  overview: TaskOverviewState,
  activeWorkflow?: string | null,
): TaskOverview[] {
  let changed = false;
  const result = tasks.flatMap((task) => {
    const record = overview.byId[task.id];
    const canonical =
      record &&
      (activeWorkflow === undefined ||
        (!record.isArchived && (!activeWorkflow || record.workflowId === activeWorkflow)))
        ? record
        : undefined;
    if (canonical !== task) changed = true;
    return canonical ? [canonical] : [];
  });
  return changed ? result : tasks;
}

function projectSnapshot<T extends KanbanState | WorkflowSnapshotData>(
  snapshot: T,
  overview: TaskOverviewState,
  previous?: T,
): T {
  const tasks = canonicalTasks(snapshot.tasks, overview, snapshot.workflowId);
  const taskIds = tasks.map((task) => task.id);
  const sameIds =
    snapshot.taskIds?.length === taskIds.length &&
    snapshot.taskIds.every((id, index) => id === taskIds[index]);
  const coverage = "taskCoverage" in snapshot ? snapshot.taskCoverage : undefined;
  const previousCoverage =
    previous && "taskCoverage" in previous ? previous.taskCoverage : undefined;
  const delta =
    previous && coverage === previousCoverage ? tasks.length - previous.tasks.length : 0;
  if (coverage && (delta || (overview.connectionGap && coverage.complete))) {
    return {
      ...snapshot,
      tasks,
      taskIds,
      taskCoverage: {
        ...coverage,
        total: coverage.total + delta,
        complete: coverage.complete && !overview.connectionGap,
      },
    };
  }
  return tasks === snapshot.tasks && sameIds ? snapshot : { ...snapshot, tasks, taskIds };
}

function projectBoards(next: AppState, overview: TaskOverviewState, previous?: AppState): AppState {
  let changed = false;
  const snapshots = Object.fromEntries(
    Object.entries(next.kanbanMulti.snapshots).map(([id, snapshot]) => {
      const projected = projectSnapshot(snapshot, overview, previous?.kanbanMulti.snapshots[id]);
      if (snapshot !== projected) changed = true;
      return [id, projected];
    }),
  );
  let archivedChanged = false;
  const archived = Object.fromEntries(
    Object.entries(next.sidebarArchivedTasks.itemsByWorkspaceId).map(([id, tasks]) => {
      const projected = canonicalTasks(tasks, overview);
      if (tasks !== projected) archivedChanged = true;
      return [id, projected];
    }),
  );
  return {
    ...next,
    taskOverview: overview,
    kanban: projectSnapshot(next.kanban, overview),
    kanbanMulti: changed ? { ...next.kanbanMulti, snapshots } : next.kanbanMulti,
    sidebarArchivedTasks: archivedChanged
      ? { ...next.sidebarArchivedTasks, itemsByWorkspaceId: archived }
      : next.sidebarArchivedTasks,
  };
}

function staleCoverage(next: AppState): AppState {
  const snapshots = Object.fromEntries(
    Object.entries(next.kanbanMulti.snapshots).map(([id, snapshot]) => [
      id,
      {
        ...snapshot,
        taskCoverage: snapshot.taskCoverage
          ? { ...snapshot.taskCoverage, complete: false }
          : undefined,
      },
    ]),
  );
  return {
    ...next,
    workflows: staleWorkflowCoverage(next.workflows),
    kanbanMulti: { ...next.kanbanMulti, snapshots },
    taskOverview: {
      ...next.taskOverview,
      reads: {},
      generation: next.taskOverview.generation + 1,
    },
  };
}

function staleWorkflowCoverage(workflows: AppState["workflows"]): AppState["workflows"] {
  const coverage = workflows.taskWorkflowCoverage;
  return coverage?.complete
    ? { ...workflows, taskWorkflowCoverage: { ...coverage, complete: false } }
    : workflows;
}

function reconcileWorkflowCoverage(previous: AppState | undefined, next: AppState): AppState {
  const coverage = next.workflows.taskWorkflowCoverage;
  if (!coverage?.complete || coverage !== previous?.workflows.taskWorkflowCoverage) return next;
  const ids = new Set(coverage.workflow_ids);
  const previousIds = new Set(previous.workflows.items.map((workflow) => workflow.id));
  const unknownWorkflow = next.workflows.items.some(
    (workflow) => workflow.workspaceId === coverage.workspace_id && !previousIds.has(workflow.id),
  );
  const unknownTasks = Object.values(next.kanbanMulti.snapshots).some(
    (snapshot) =>
      !ids.has(snapshot.workflowId) &&
      snapshot.tasks.some((task) => task.workspaceId === coverage.workspace_id),
  );
  return unknownWorkflow || unknownTasks
    ? {
        ...next,
        workflows: {
          ...next.workflows,
          taskWorkflowCoverage: { ...coverage, complete: false },
        },
      }
    : next;
}

function isolateOverviewSources(previous: AppState | undefined, next: AppState): AppState {
  if (!previous) return next;
  const workspaceId = next.workspaces.activeId;
  const keep = (tasks: TaskOverview[], old: TaskOverview[] | undefined) =>
    tasks === old
      ? []
      : tasks.filter(
          (task) =>
            task.workspaceId === workspaceId ||
            (!task.workspaceId &&
              next.workflows.items.some(
                (workflow) =>
                  workflow.id === task.workflowId && workflow.workspaceId === workspaceId,
              )),
        );
  return {
    ...next,
    kanban: { ...next.kanban, tasks: keep(next.kanban.tasks, previous.kanban.tasks) },
    kanbanMulti: {
      ...next.kanbanMulti,
      snapshots: Object.fromEntries(
        Object.entries(next.kanbanMulti.snapshots).map(([id, snapshot]) => [
          id,
          {
            ...snapshot,
            tasks: keep(snapshot.tasks, previous.kanbanMulti.snapshots[id]?.tasks),
            taskCoverage:
              snapshot === previous.kanbanMulti.snapshots[id] ||
              snapshot.taskCoverage?.workspace_id !== workspaceId
                ? undefined
                : snapshot.taskCoverage,
          },
        ]),
      ),
    },
    sidebarArchivedTasks: { ...next.sidebarArchivedTasks, itemsByWorkspaceId: {} },
  };
}

function overviewRecovery(previous: AppState | undefined, state: AppState) {
  const disconnected =
    previous?.connection.status === "connected" && state.connection.status !== "connected";
  const reconnected =
    previous?.taskOverview.connectionGap === true && state.connection.status === "connected";
  const connectionGap =
    disconnected || (previous?.taskOverview.connectionGap === true && !reconnected);
  const overflowed = previous && previous.taskOverview.generation !== state.taskOverview.generation;
  return { connectionGap, invalidated: disconnected || reconnected || overflowed };
}

export function normalizeTaskOverviews(previous: AppState | undefined, state: AppState): AppState {
  const scope = taskOverviewScope(state);
  const scopeChanged = previous?.taskOverview.scope !== scope;
  const { connectionGap, invalidated } = overviewRecovery(previous, state);
  let next = reconcileWorkflowCoverage(previous, invalidated ? staleCoverage(state) : state);
  if (connectionGap) next = { ...next, workflows: staleWorkflowCoverage(next.workflows) };
  if (scopeChanged) next = isolateOverviewSources(previous, next);
  if (!scopeChanged && !overviewInputsChanged(previous, next)) return next;
  const base = scopeChanged
    ? emptyTaskOverviewState(scope, (previous?.taskOverview.generation ?? 0) + 1)
    : next.taskOverview;
  const overview = { ...base, connectionGap, byId: { ...base.byId }, owners: { ...base.owners } };
  collectBoardOwners(scopeChanged ? undefined : previous, next, overview);
  if (overview.generation !== base.generation) {
    next = staleCoverage(next);
    overview.reads = {};
  }
  releaseUnownedRecords(overview);
  return projectBoards(next, overview, previous);
}
