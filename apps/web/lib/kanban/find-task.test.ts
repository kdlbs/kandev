import { describe, it, expect } from "vitest";
import { findTaskInSnapshots } from "./find-task";
import type { KanbanState } from "@/lib/state/slices";

type Task = KanbanState["tasks"][number];

function task(id: string): Task {
  return { id, workflowStepId: "s", title: id, position: 0 };
}

describe("findTaskInSnapshots", () => {
  it("finds a task in a snapshot", () => {
    const snap = { tasks: [task("t1"), task("t2")] };
    expect(findTaskInSnapshots("t2", { wf: snap })?.id).toBe("t2");
  });

  it("searches across every loaded snapshot", () => {
    const snaps = { wfA: { tasks: [task("t1")] }, wfB: { tasks: [task("t2")] } };
    expect(findTaskInSnapshots("t2", snaps)?.id).toBe("t2");
  });

  it("returns the snapshot match when present", () => {
    const snap = { tasks: [{ ...task("t1"), title: "from-snapshot" }] };
    expect(findTaskInSnapshots("t1", { wf: snap })?.title).toBe("from-snapshot");
  });

  it("returns null when the task is missing from snapshots", () => {
    expect(findTaskInSnapshots("missing", { wf: { tasks: [task("t1")] } })).toBeNull();
    expect(findTaskInSnapshots("missing", {})).toBeNull();
  });
});
