import { describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateState } from "./hydrator";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";

// Testing round (post-Build-round-3): a REST refresh
// (fetchWorkflowSnapshot -> snapshotToState -> hydrate) can resolve after a
// fresher WS task.updated already landed. mergeKanbanTasks decides which
// whole task object wins on `updatedAt`, but parked state is never
// persisted (D1) so it never touches `updated_at` — without its own discard
// rule, a stale REST snapshot can regress a live parked value even though
// its task object "wins" the updatedAt comparison. Split into its own file
// (rather than hydrator.test.ts) to keep that file under its 600-line limit.

function makeAppDraft(): AppState {
  return structuredClone(defaultState) as AppState;
}

function taskWith(overrides: Record<string, unknown>) {
  return {
    id: "t1",
    workflowStepId: "step1",
    title: "Task",
    position: 0,
    updatedAt: "2026-01-01T00:00:00.000Z",
    ...overrides,
  };
}

describe("hydrateState — kanban task parked projection ordering (D1, AC-39, AC-49)", () => {
  it("discards a stale parked triple carried on a REST snapshot task that otherwise wins on updatedAt", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.kanban.tasks = [
        taskWith({
          updatedAt: "2026-01-01T00:00:00.000Z",
          parkedOnBackgroundWork: true,
          parkedEpoch: 100,
          parkedRevision: 7,
        }),
      ] as AppState["kanban"]["tasks"];
      hydrateState(draft, {
        kanban: {
          workflowId: "wf1",
          steps: [],
          tasks: [
            taskWith({
              updatedAt: "2026-01-02T00:00:00.000Z",
              parkedOnBackgroundWork: false,
              parkedEpoch: 100,
              parkedRevision: 6,
            }),
          ],
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.kanban.tasks[0]).toMatchObject({
      parkedOnBackgroundWork: true,
      parkedEpoch: 100,
      parkedRevision: 7,
    });
  });

  it("accepts a fresher parked triple carried on a REST snapshot task", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.kanban.tasks = [
        taskWith({
          parkedOnBackgroundWork: false,
          parkedEpoch: 100,
          parkedRevision: 1,
        }),
      ] as AppState["kanban"]["tasks"];
      hydrateState(draft, {
        kanban: {
          workflowId: "wf1",
          steps: [],
          tasks: [
            taskWith({
              parkedOnBackgroundWork: true,
              parkedEpoch: 100,
              parkedRevision: 2,
            }),
          ],
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.kanban.tasks[0]).toMatchObject({
      parkedOnBackgroundWork: true,
      parkedEpoch: 100,
      parkedRevision: 2,
    });
  });
});
