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

const FACET = "facet:plugin:tags";

type FacetNode = { task: Task; children: FacetNode[]; level: number };

function flatten(nodes: FacetNode[]): FacetNode[] {
  return nodes.flatMap((node) => [node, ...flatten(node.children)]);
}

function facetSections(
  tasks: Task[],
  values: Record<string, Array<{ value: string; label: string; color?: string }>>,
) {
  return buildTaskSections(tasks, {
    groupBy: FACET,
    workflowMap: new Map(),
    repoMap: new Map(),
    facetValues: values,
  });
}

function facetTask(id: string): Task {
  return { id, title: id, state: "REVIEW" } as Task;
}

describe("facet value sections", () => {
  // @covers AC-PLUGINS-TASKLIST-FACETS-002.1 and 002.2
  it("includes only carried values, keeps per-bucket trees, color and multi-membership", () => {
    const parent = facetTask("parent");
    const child = { ...facetTask("child"), parent_id: "parent" } as Task;
    const result = facetSections([parent, child], {
      [`${FACET}:parent`]: [
        { value: "alpha", label: "Alpha", color: "#abcdef" },
        { value: "beta", label: "Beta" },
        { value: "beta", label: "Beta" },
      ],
      [`${FACET}:child`]: [{ value: "alpha", label: "Alpha" }],
      [`${FACET}:other`]: [{ value: "unused", label: "Not on page" }],
    });
    expect(result.map((section) => section.key)).toEqual(["facet:value:alpha", "facet:value:beta"]);
    expect(result[0].color).toBe("#abcdef");
    expect(flatten(result[0].nodes).map((node) => [node.task.id, node.level])).toEqual([
      ["parent", 0],
      ["child", 1],
    ]);
    expect(result[0].nodes[0].children.map((node) => node.task.id)).toEqual(["child"]);
    // A value the plugin returns twice for one task is one row, not two.
    expect(result[1].nodes.map((node) => node.task.id)).toEqual(["parent"]);
  });

  // @covers AC-PLUGINS-TASKLIST-FACETS-002.3 and 002.4
  it("sorts labels case-insensitively, retains same-base input order, then appends Unassigned", () => {
    const [a, b, c] = [facetTask("a"), facetTask("b"), facetTask("c")];
    const result = facetSections([a, b, c], {
      [`${FACET}:a`]: [{ value: "upper", label: "Beta" }],
      [`${FACET}:b`]: [{ value: "lower", label: "beta" }],
      [`${FACET}:c`]: [{ value: "empty", label: "" }],
    });
    expect(result.map((section) => [section.key, section.title])).toEqual([
      ["facet:value:upper", "Beta"],
      ["facet:value:lower", "beta"],
      ["facet:host:ungrouped", "Unassigned"],
    ]);
    expect(result[2].nodes.map((node) => node.task.id)).toEqual(["c"]);
  });

  // @covers AC-PLUGINS-TASKLIST-FACETS-002.8
  it("keeps valid sibling labels and namespaced host keys separate from plugin values", () => {
    const [a, b] = [facetTask("a"), facetTask("b")];
    const result = facetSections([a, b], {
      [`${FACET}:a`]: [
        { value: "missing", label: "" },
        { value: "__host_ungrouped__", label: "Zulu" },
      ],
    });
    expect(result.map((section) => section.key)).toEqual([
      "facet:value:__host_ungrouped__",
      "facet:host:ungrouped",
    ]);
    expect(result[0].nodes.map((node) => node.task.id)).toEqual(["a"]);
    expect(result[1].nodes.map((node) => node.task.id)).toEqual(["b"]);
  });

  // AC-PLUGINS-TASKLIST-FACETS-002.4: a child without its parent in a value bucket is a root.
  it("renders a child at the section root when its parent carries another value", () => {
    const parent = facetTask("parent");
    const child = { ...facetTask("child"), parent_id: "parent" } as Task;
    const result = facetSections([parent, child], {
      [`${FACET}:parent`]: [{ value: "alpha", label: "Alpha" }],
      [`${FACET}:child`]: [{ value: "beta", label: "Beta" }],
    });

    expect(result.map((section) => section.title)).toEqual(["Alpha", "Beta"]);
    expect(result[0].nodes.map((node) => [node.task.id, node.level])).toEqual([["parent", 0]]);
    expect(result[1].nodes.map((node) => [node.task.id, node.level])).toEqual([["child", 0]]);
  });
});
