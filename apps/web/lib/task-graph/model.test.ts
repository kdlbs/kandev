import { describe, expect, it } from "vitest";
import type { TaskGraphResponse } from "@/lib/api/domains/task-graph-api";
import { buildTaskGraphModel, traceChain } from "./model";

function node(id: string, role: "member" | "external" = "member") {
  return { id, title: id, state: "TODO", depth: 1, role, archived: false, blocked: false };
}

function response(
  ids: string[],
  edges: Array<[string, string]>,
  extra: Partial<TaskGraphResponse> = {},
): TaskGraphResponse {
  return {
    mode: "subtasks",
    available_modes: ["subtasks", "chain"],
    focus_task_id: ids[0],
    nodes: ids.map((id) => node(id)),
    edges: edges.map(([from, to]) => ({
      from,
      to,
      kind: "dependency" as const,
      status: "pending" as const,
    })),
    truncated: false,
    ...extra,
  };
}

function steps(model: ReturnType<typeof buildTaskGraphModel>) {
  return Object.fromEntries(model.nodes.map((n) => [n.id, n.step]));
}

describe("buildTaskGraphModel", () => {
  it("numbers steps by the longest predecessor path", () => {
    // a → b → d, a → c, c → d, e alone
    const model = buildTaskGraphModel(
      response(
        ["a", "b", "c", "d", "e"],
        [
          ["a", "b"],
          ["b", "d"],
          ["a", "c"],
          ["c", "d"],
        ],
      ),
    );
    expect(steps(model)).toEqual({ a: 1, b: 2, c: 2, d: 3, e: 1 });
    expect(model.noOrderSet).toBe(false);
  });

  it("ignores edges whose ends are not displayed", () => {
    const model = buildTaskGraphModel(
      response(
        ["a", "b"],
        [
          ["ghost", "b"],
          ["a", "b"],
        ],
      ),
    );
    expect(model.edges).toHaveLength(1);
    expect(steps(model)).toEqual({ a: 1, b: 2 });
  });

  it("terminates on a corrupt cycle", () => {
    const model = buildTaskGraphModel(
      response(
        ["a", "b", "c"],
        [
          ["a", "b"],
          ["b", "c"],
          ["c", "b"],
        ],
      ),
    );
    expect(model.nodes.find((n) => n.id === "a")?.step).toBe(1);
    for (const id of ["b", "c"]) {
      expect(model.nodes.find((n) => n.id === id)?.step).toBeGreaterThan(1);
    }
  });

  it("flags subtasks with no order between members", () => {
    const res = response(["a", "b", "x"], [["x", "a"]]);
    res.nodes[2] = node("x", "external");
    expect(buildTaskGraphModel(res).noOrderSet).toBe(true);
    expect(buildTaskGraphModel(response(["a", "b"], [["a", "b"]])).noOrderSet).toBe(false);
  });

  it("keeps related edges without using them to compute steps or order", () => {
    const model = buildTaskGraphModel(
      response(["a", "b", "c"], [], {
        edges: [
          { from: "a", to: "c", kind: "dependency", status: "pending" },
          { from: "a", to: "b", kind: "related" },
        ],
      }),
    );
    expect(steps(model)).toEqual({ a: 1, b: 1, c: 2 });
    expect(model.edges).toHaveLength(2);
    expect(model.predecessors.get("b")).toEqual([]);
  });

  it("keeps dependency and related links between the same pair distinct", () => {
    const model = buildTaskGraphModel(
      response(["a", "b"], [], {
        edges: [
          { from: "a", to: "b", kind: "dependency", status: "pending" },
          { from: "a", to: "b", kind: "related" },
        ],
      }),
    );
    expect(new Set(model.edges.map((edge) => edge.id)).size).toBe(2);
  });

  it("never flags the chain view as unordered", () => {
    const model = buildTaskGraphModel(response(["a"], [], { mode: "chain" }));
    expect(model.noOrderSet).toBe(false);
  });

  it("orders nodes by step, then discovery", () => {
    const model = buildTaskGraphModel(response(["c", "b", "a"], [["a", "c"]]));
    expect(model.nodes.map((n) => n.id)).toEqual(["b", "a", "c"]);
  });
});

describe("traceChain", () => {
  it("returns transitive predecessors, dependents and the edges between them", () => {
    // a → b → c, s → c, b → d
    const model = buildTaskGraphModel(
      response(
        ["a", "b", "c", "d", "s", "z"],
        [
          ["a", "b"],
          ["b", "c"],
          ["s", "c"],
          ["b", "d"],
        ],
      ),
    );
    const chain = traceChain(model, "b");
    expect([...chain.nodeIds].sort()).toEqual(["a", "b", "c", "d"]);
    expect([...chain.edgeIds].sort()).toEqual([
      "dependency:a->b",
      "dependency:b->c",
      "dependency:b->d",
    ]);
  });
});
