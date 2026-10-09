import { describe, expect, it } from "vitest";
import type { Task, Workflow } from "@/lib/types/http";
import { buildTaskSections, type TaskListStepPreviews } from "./task-list-sections";

function task(id: string, workflow: string, step: string, overrides: Partial<Task> = {}): Task {
  return {
    id,
    title: id,
    workflow_id: workflow,
    workflow_step_id: step,
    state: "REVIEW",
    ...overrides,
  } as Task;
}
const workflows = [
  { id: "wf-b", name: "Second", sort_order: 1 },
  { id: "wf-a", name: "First", sort_order: 0 },
] as Workflow[];
const workflowStepPreviews: TaskListStepPreviews = {
  "wf-a": {
    status: "success",
    steps: [
      { id: "a-review", title: "Review", position: 1 },
      { id: "a-build", title: "Build", position: 0 },
    ],
  },
  "wf-b": { status: "success", steps: [{ id: "b-review", title: "Review", position: 0 }] },
};
function sections(tasks: Task[], previews = workflowStepPreviews) {
  return buildTaskSections(tasks, {
    groupBy: "workflow_step",
    workflows,
    workflowStepPreviews: previews,
    workflowMap: new Map(),
    repoMap: new Map(),
    facetValues: {},
  });
}

// @covers AC-UI-LIST-STEP-GROUPING-001.2 and AC-UI-LIST-STEP-GROUPING-001.3
describe("workflow step sections", () => {
  it("keeps different steps separate despite identical states or names and follows workflow/step order", () => {
    const grouped = sections([
      task("second", "wf-b", "b-review"),
      task("review", "wf-a", "a-review"),
      task("build", "wf-a", "a-build"),
    ]);
    expect(grouped.map((section) => section.title)).toEqual([
      "First / Build",
      "First / Review",
      "Second / Review",
    ]);
    expect(grouped.map((section) => section.nodes[0].task.id)).toEqual([
      "build",
      "review",
      "second",
    ]);
    expect(new Set(grouped.map((section) => section.key)).size).toBe(3);
  });

  it("groups the same step despite different states and keeps task order", () => {
    const grouped = sections([
      task("waiting", "wf-a", "a-build", { state: "WAITING_FOR_INPUT" }),
      task("completed", "wf-a", "a-build", { state: "COMPLETED" }),
    ]);
    expect(grouped).toHaveLength(1);
    expect(grouped[0].title).toBe("Build");
    expect(grouped[0].nodes.map((node) => node.task.id)).toEqual(["waiting", "completed"]);
  });

  // @covers AC-UI-LIST-STEP-GROUPING-001.4
  it("preserves distinct unavailable step identities and puts no-step tasks last", () => {
    const grouped = sections([
      task("none", "wf-a", ""),
      task("missing", "wf-a", "removed"),
      task("other", "wf-a", "another-removed"),
      task("known", "wf-a", "a-build"),
    ]);
    expect(grouped.map((section) => section.title)).toEqual([
      "Build",
      "Unknown workflow step",
      "Unknown workflow step",
      "No workflow step",
    ]);
    expect(new Set(grouped.map((section) => section.key)).size).toBe(4);
  });

  it("retains rows during metadata loading and failure then resolves their names", () => {
    const tasks = [task("work", "wf-a", "a-build")];
    expect(sections(tasks, { "wf-a": { status: "loading" } })[0].title).toBe(
      "Loading workflow step...",
    );
    expect(sections(tasks, { "wf-a": { status: "error" } })[0].nodes[0].task.id).toBe("work");
    expect(sections(tasks)[0].title).toBe("Build");
  });

  // @covers AC-UI-LIST-STEP-GROUPING-001.6
  it("keeps displayed children with their parent and uses an absent parent's child's own step", () => {
    const parent = task("parent", "wf-a", "a-build");
    const child = task("child", "wf-a", "a-review", { parent_id: parent.id });
    const grouped = sections([parent, child]);
    expect(grouped).toHaveLength(1);
    expect(grouped[0].nodes[0].children[0]).toMatchObject({ level: 1, task: { id: "child" } });
    expect(sections([child])[0]).toMatchObject({ title: "Review", nodes: [{ level: 0 }] });
  });
});
