import { describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateState } from "./hydrator";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";

// Testing round (post-Build-round-4): mergeKanbanTasks previously did
// `draftTasks[idx] = incoming; applyParkedTriple(draftTasks[idx], ...)`.
// Immer does not re-wrap a value assigned within the current producer, so
// that second line mutated the caller's own `incoming` object in place — a
// hydrateState caller's `source` task objects must never be touched, since
// some callers (use-all-workflow-snapshots.ts's fetchAndWriteSnapshot) share
// those exact object references with other parts of the store
// (kanbanMulti.snapshots) that hydrateState has no business touching. Split
// into its own file (rather than hydrator-kanban-parked.test.ts) to keep
// that file's describe block under its 100-line function limit.

const UPDATED_AT_OLD = "2026-01-01T00:00:00.000Z";
const UPDATED_AT_NEW = "2026-01-02T00:00:00.000Z";

function makeAppDraft(): AppState {
  return structuredClone(defaultState) as AppState;
}

function taskWith(overrides: Record<string, unknown>) {
  return {
    id: "t1",
    workflowStepId: "step1",
    title: "Task",
    position: 0,
    updatedAt: UPDATED_AT_OLD,
    ...overrides,
  };
}

describe("hydrateState — kanban task parked merge does not mutate its inputs", () => {
  it("does not mutate the caller's incoming task object in place", () => {
    const draft0 = makeAppDraft();
    draft0.kanban.tasks = [
      taskWith({
        updatedAt: UPDATED_AT_OLD,
        parkedOnBackgroundWork: true,
        parkedEpoch: 100,
        parkedRevision: 7,
      }),
    ] as AppState["kanban"]["tasks"];

    const incomingTask = taskWith({
      updatedAt: UPDATED_AT_NEW,
      parkedOnBackgroundWork: false,
      parkedEpoch: 100,
      parkedRevision: 6,
    });
    const incomingTaskSnapshot = { ...incomingTask };

    produce(draft0, (draft: Draft<AppState>) => {
      hydrateState(draft, {
        kanban: { workflowId: "wf1", steps: [], tasks: [incomingTask] },
      } as unknown as Partial<AppState>);
    });

    expect(incomingTask).toEqual(incomingTaskSnapshot);
  });

  // use-task-crud.ts's delete/archive handlers pass store.getState().kanban.tasks
  // itself as the incoming `source` array, so for every retained task
  // `existing === incoming` (the literal same reference already in the
  // draft's base state) — a second reachable shape of the same aliasing risk.
  it("does not corrupt a task when the incoming object is the same reference as the existing one", () => {
    const sharedTask = taskWith({
      updatedAt: UPDATED_AT_OLD,
      parkedOnBackgroundWork: true,
      parkedEpoch: 100,
      parkedRevision: 7,
    });
    const draft0 = makeAppDraft();
    draft0.kanban.tasks = [sharedTask] as AppState["kanban"]["tasks"];

    const result = produce(draft0, (draft: Draft<AppState>) => {
      hydrateState(draft, {
        kanban: { workflowId: "wf1", steps: [], tasks: [sharedTask] },
      } as unknown as Partial<AppState>);
    });

    expect(result.kanban.tasks[0]).toMatchObject({
      parkedOnBackgroundWork: true,
      parkedEpoch: 100,
      parkedRevision: 7,
    });
    expect(sharedTask).toMatchObject({
      parkedOnBackgroundWork: true,
      parkedEpoch: 100,
      parkedRevision: 7,
    });
  });
});
