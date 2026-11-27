import { describe, expect, it } from "vitest";
import type { WorkflowSnapshot } from "@/lib/types/http";
import {
  mapSnapshotToKanban,
  reconcileMobileSnapshot,
} from "./session-task-switcher-sheet-helpers";
import { createAppStore } from "@/lib/state/store";
import { recordTaskOverviewChange } from "@/lib/state/slices/task-overview-merge";

describe("mapSnapshotToKanban", () => {
  it("preserves the signal-gated flag when switching workflows", () => {
    const snapshot = {
      steps: [
        {
          id: "step-1",
          name: "Review",
          position: 1,
          color: "bg-blue-500",
          auto_advance_requires_signal: true,
        },
      ],
      tasks: [],
    } as unknown as WorkflowSnapshot;

    const state = mapSnapshotToKanban(snapshot, "workflow-1");

    expect(state.steps[0]).toMatchObject({
      auto_advance_requires_signal: true,
    });
  });
});

function coveredSnapshot(): WorkflowSnapshot {
  return {
    steps: [{ id: "step", name: "Step", position: 0 }],
    tasks: [
      {
        id: "task",
        workspace_id: "ws",
        workflow_id: "wf",
        workflow_step_id: "step",
        title: "Original",
      },
    ],
    task_coverage: {
      workspace_id: "ws",
      workflow_id: "wf",
      membership: "active",
      total: 1,
      complete: true,
      ordering_profile: "sqlite_nocase_v1",
    },
  } as WorkflowSnapshot;
}

it("does not claim complete mobile coverage when a snapshot omits a task's step", () => {
  const snapshot = coveredSnapshot();
  snapshot.steps = [];
  const mapped = mapSnapshotToKanban(snapshot, "wf");
  expect(mapped.tasks).toEqual([]);
  expect(mapped.taskCoverage?.complete).toBe(false);
});

it("reconciles mobile reads with live deletions and rejects a previous workspace", () => {
  const store = createAppStore();
  store.getState().setActiveWorkspace("ws");
  const read = store.getState().beginTaskOverviewRead();
  const snapshot = mapSnapshotToKanban(coveredSnapshot(), "wf");
  store.setState((state) => ({
    taskOverview: recordTaskOverviewChange(state.taskOverview, "task", null),
  }));
  const current = reconcileMobileSnapshot(store.getState(), snapshot, read);
  expect(current?.tasks).toEqual([]);
  expect(current?.taskCoverage).toMatchObject({ complete: true, total: 0 });
  store.getState().setActiveWorkspace("other");
  expect(reconcileMobileSnapshot(store.getState(), snapshot, read)).toBeNull();
});
