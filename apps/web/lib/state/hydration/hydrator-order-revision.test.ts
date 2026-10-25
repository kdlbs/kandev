import { describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateState } from "./hydrator";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";

function makeAppDraft(): AppState {
  return structuredClone(defaultState) as AppState;
}

const STEP_COLOR = "bg-neutral-400";

/**
 * Covers the Build-phase fix for missing order_revision on HTTP hydration:
 * an unsolicited task.reordered WS event arriving right after a page load
 * must be compared against the step's real last-known revision, not the
 * "no revision recorded yet" (-1) fallback that would accept any event as
 * the first order ever seen.
 */
describe("hydrateState — kanbanMulti.orderRevisionByStepId seeding", () => {
  it("seeds a step's revision from kanban.steps on the initial boot hydration", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      hydrateState(draft, {
        kanban: {
          workflowId: "wf-1",
          steps: [
            {
              id: "step-1",
              title: "Step 1",
              color: STEP_COLOR,
              position: 0,
              order_revision: 4,
            },
          ],
          tasks: [],
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.kanbanMulti.orderRevisionByStepId["step-1"]).toBe(4);
  });

  it("seeds revisions from every workflow's snapshot, not only the active one", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      hydrateState(draft, {
        kanbanMulti: {
          snapshots: {
            "wf-background": {
              workflowId: "wf-background",
              workflowName: "Background",
              steps: [
                {
                  id: "step-bg",
                  title: "Bg",
                  color: STEP_COLOR,
                  position: 0,
                  order_revision: 2,
                },
              ],
              tasks: [],
            },
          },
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.kanbanMulti.orderRevisionByStepId["step-bg"]).toBe(2);
  });

  it("never rolls back a fresher recorded revision to a stale hydrated one", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.kanbanMulti.orderRevisionByStepId["step-1"] = 9;

      hydrateState(draft, {
        kanban: {
          workflowId: "wf-1",
          steps: [
            {
              id: "step-1",
              title: "Step 1",
              color: STEP_COLOR,
              position: 0,
              order_revision: 3,
            },
          ],
          tasks: [],
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.kanbanMulti.orderRevisionByStepId["step-1"]).toBe(9);
  });

  it("leaves the revision map untouched when a step omits order_revision", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      hydrateState(draft, {
        kanban: {
          workflowId: "wf-1",
          steps: [{ id: "step-1", title: "Step 1", color: STEP_COLOR, position: 0 }],
          tasks: [],
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.kanbanMulti.orderRevisionByStepId["step-1"]).toBeUndefined();
  });
});
