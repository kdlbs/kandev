import { describe, expect, it } from "vitest";
import type { TaskGraphResponse } from "@/lib/api/domains/task-graph-api";
import { buildTaskGraphModel } from "./model";
import {
  TASK_GRAPH_NODE_HEIGHT,
  TASK_GRAPH_NODE_WIDTH,
  layoutGroupedTaskGraph,
  layoutTaskGraph,
  stepFrames,
} from "./layout";

function model(ids: string[], edges: Array<[string, string]>) {
  const response: TaskGraphResponse = {
    mode: "chain",
    available_modes: ["chain"],
    focus_task_id: ids[0],
    nodes: ids.map((id) => ({
      id,
      title: id,
      state: "TODO",
      depth: 0,
      role: "member",
      archived: false,
      blocked: false,
    })),
    edges: edges.map(([from, to]) => ({ from, to, kind: "dependency", status: "pending" })),
    truncated: false,
  };
  return buildTaskGraphModel(response);
}

describe("layoutTaskGraph", () => {
  it("positions every node with the shared card size", () => {
    const layout = layoutTaskGraph(model(["a", "b"], [["a", "b"]]));
    for (const id of ["a", "b"]) {
      expect(layout.get(id)).toMatchObject({
        width: TASK_GRAPH_NODE_WIDTH,
        height: TASK_GRAPH_NODE_HEIGHT,
      });
    }
  });

  it("puts each step in its own column, left to right", () => {
    // s has a single far successor; dagre alone may pull it rightwards.
    const m = model(
      ["a", "b", "c", "s"],
      [
        ["a", "b"],
        ["b", "c"],
        ["s", "c"],
      ],
    );
    const layout = layoutTaskGraph(m);
    const x = (id: string) => layout.get(id)?.x ?? NaN;
    expect(x("a")).toBe(x("s"));
    expect(x("a")).toBeLessThan(x("b"));
    expect(x("b")).toBeLessThan(x("c"));
  });

  it("does not overlap nodes in the same column", () => {
    const layout = layoutTaskGraph(model(["a", "b", "c"], []));
    const ys = ["a", "b", "c"].map((id) => layout.get(id)?.y ?? 0).sort((p, q) => p - q);
    expect(ys[1] - ys[0]).toBeGreaterThanOrEqual(TASK_GRAPH_NODE_HEIGHT);
    expect(ys[2] - ys[1]).toBeGreaterThanOrEqual(TASK_GRAPH_NODE_HEIGHT);
  });
});

describe("layoutGroupedTaskGraph", () => {
  it("does not let a non-blocking related edge shift horizontal rank", () => {
    const response: TaskGraphResponse = {
      mode: "chain",
      available_modes: ["chain"],
      focus_task_id: "a",
      nodes: ["a", "b"].map((id) => ({
        id,
        title: id,
        state: "TODO",
        depth: 0,
        role: "member",
        archived: false,
        blocked: false,
      })),
      edges: [{ from: "a", to: "b", kind: "related", status: "pending" }],
      truncated: false,
    };
    const m = buildTaskGraphModel(response);
    const { nodes } = layoutGroupedTaskGraph(m, []);
    expect(nodes.get("a")?.x).toBe(nodes.get("b")?.x);
  });

  it("frames each group around its members without overlapping other nodes", () => {
    const m = model(["p", "x", "y", "solo"], [["x", "y"]]);
    const { nodes, groups } = layoutGroupedTaskGraph(m, [{ id: "group:p", memberIds: ["x", "y"] }]);
    const frame = groups.get("group:p");
    expect(frame).toBeDefined();
    for (const id of ["x", "y"]) {
      const box = nodes.get(id)!;
      expect(box.x).toBeGreaterThanOrEqual(frame!.x);
      expect(box.y).toBeGreaterThanOrEqual(frame!.y);
      expect(box.x + box.width).toBeLessThanOrEqual(frame!.x + frame!.width);
      expect(box.y + box.height).toBeLessThanOrEqual(frame!.y + frame!.height);
    }
    for (const id of ["p", "solo"]) {
      const box = nodes.get(id)!;
      const inside =
        box.x >= frame!.x &&
        box.y >= frame!.y &&
        box.x + box.width <= frame!.x + frame!.width &&
        box.y + box.height <= frame!.y + frame!.height;
      expect(inside).toBe(false);
    }
  });
});

describe("stepFrames", () => {
  it("bands each step column and counts the tasks that can run in parallel", () => {
    // a → c, b → c: a and b share step 1.
    const m = model(
      ["a", "b", "c"],
      [
        ["a", "c"],
        ["b", "c"],
      ],
    );
    const layout = layoutTaskGraph(m);
    const frames = stepFrames(m, layout);
    expect(frames.map((f) => [f.step, f.count])).toEqual([
      [1, 2],
      [2, 1],
    ]);
    const one = frames[0].box;
    for (const id of ["a", "b"]) {
      const box = layout.get(id)!;
      expect(box.x).toBeGreaterThanOrEqual(one.x);
      expect(box.y).toBeGreaterThanOrEqual(one.y);
      expect(box.x + box.width).toBeLessThanOrEqual(one.x + one.width);
    }
    expect(frames[1].box.x).toBeGreaterThan(one.x + one.width - 1);
    // Shared top and bottom: labels line up above every card.
    expect(frames[1].box.y).toBe(one.y);
    expect(frames[1].box.height).toBe(one.height);
  });
});
