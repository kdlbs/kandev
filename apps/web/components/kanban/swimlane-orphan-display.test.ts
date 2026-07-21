import { describe, expect, it } from "vitest";
import { remapOrphanTasks } from "./swimlane-kanban-content";

const ORPHAN_ID = "__kandev_orphan__";

function makeTask(id: string, stepId: string) {
  return { id, workflowStepId: stepId } as Parameters<typeof remapOrphanTasks>[0][number];
}

describe("remapOrphanTasks", () => {
  it("leaves tasks on known steps unchanged", () => {
    const stepIds = new Set(["s1", "s2"]);
    const tasks = [makeTask("t1", "s1"), makeTask("t2", "s2")];
    const { tasks: out, hasOrphans } = remapOrphanTasks(tasks, stepIds, ORPHAN_ID);
    expect(hasOrphans).toBe(false);
    expect(out[0].workflowStepId).toBe("s1");
    expect(out[1].workflowStepId).toBe("s2");
  });

  it("remaps a task with a dead step to the orphan sentinel", () => {
    const stepIds = new Set(["s1"]);
    const tasks = [makeTask("t-orphan", "deleted-step")];
    const { tasks: out, hasOrphans } = remapOrphanTasks(tasks, stepIds, ORPHAN_ID);
    expect(hasOrphans).toBe(true);
    expect(out[0].workflowStepId).toBe(ORPHAN_ID);
  });

  it("does not mutate the original task object", () => {
    const stepIds = new Set(["s1"]);
    const original = makeTask("t-orig", "dead");
    const { tasks: out } = remapOrphanTasks([original], stepIds, ORPHAN_ID);
    // Original must not change.
    expect(original.workflowStepId).toBe("dead");
    // Returned task is a new object.
    expect(out[0]).not.toBe(original);
  });

  it("handles a mix of valid and orphaned tasks", () => {
    const stepIds = new Set(["s1", "s2"]);
    const tasks = [makeTask("t-ok1", "s1"), makeTask("t-orphan", "ghost"), makeTask("t-ok2", "s2")];
    const { tasks: out, hasOrphans } = remapOrphanTasks(tasks, stepIds, ORPHAN_ID);
    expect(hasOrphans).toBe(true);
    expect(out[0].workflowStepId).toBe("s1");
    expect(out[1].workflowStepId).toBe(ORPHAN_ID);
    expect(out[2].workflowStepId).toBe("s2");
  });

  it("does not remap tasks with an empty workflowStepId", () => {
    const stepIds = new Set(["s1"]);
    const tasks = [makeTask("t-empty", "")];
    const { tasks: out, hasOrphans } = remapOrphanTasks(tasks, stepIds, ORPHAN_ID);
    expect(hasOrphans).toBe(false);
    expect(out[0].workflowStepId).toBe("");
  });

  it("returns hasOrphans=false for an empty task list", () => {
    const { hasOrphans } = remapOrphanTasks([], new Set(["s1"]), ORPHAN_ID);
    expect(hasOrphans).toBe(false);
  });
});
