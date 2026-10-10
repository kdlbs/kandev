import { expect, it } from "vitest";
import { createAppStore } from "../store";
import { recordTaskOverviewChange } from "./task-overview-merge";
import { reconcileTaskWorkflowCoverage } from "./task-workflow-coverage";

const coverage = { workspace_id: "ws", workflow_ids: ["a"], complete: true };

it("reconciles scopes added during a workflow read without inventing completeness", () => {
  const store = createAppStore();
  store.getState().setActiveWorkspace("ws");
  const read = store.getState().beginTaskOverviewRead();
  for (const [id, workflowId] of [
    ["created", "hidden"],
    ["unassigned", ""],
  ])
    store.setState((state) => ({
      taskOverview: recordTaskOverviewChange(state.taskOverview, id, { id, workflowId }),
    }));
  store.setState((state) => ({
    taskOverview: recordTaskOverviewChange(state.taskOverview, "deleted", null),
  }));
  expect(reconcileTaskWorkflowCoverage(store.getState(), coverage, read)).toEqual({
    ...coverage,
    workflow_ids: ["a", "hidden", ""],
  });
  expect(
    reconcileTaskWorkflowCoverage(store.getState(), { ...coverage, complete: false }, read)
      ?.complete,
  ).toBe(false);
});

it("fails closed when a context change or overflow has expired the workflow read", () => {
  const store = createAppStore();
  const read = store.getState().beginTaskOverviewRead();
  store.getState().setActiveWorkspace("other");
  expect(reconcileTaskWorkflowCoverage(store.getState(), coverage, read)?.complete).toBe(false);
  expect(reconcileTaskWorkflowCoverage(store.getState(), undefined, read)).toBeUndefined();
});
