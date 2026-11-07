import { describe, expect, it } from "vitest";
import { buildBoardGraph, type BoardGraphTask } from "./board";

function task(id: string, extra: Partial<BoardGraphTask> = {}): BoardGraphTask {
  return { id, title: `T ${id}`, state: "TODO", ...extra };
}

describe("buildBoardGraph", () => {
  it("turns board tasks into nodes and in-board dependency edges", () => {
    const { response } = buildBoardGraph(
      [
        task("a"),
        task("b", { dependsOn: [{ id: "a", status: "resolved" }, { id: "elsewhere" }] }),
        task("c", { blocked: true, blockedReason: "pending" }),
      ],
      "b",
    );
    expect(response.nodes.map((n) => n.id)).toEqual(["a", "b", "c"]);
    expect(response.edges).toEqual([
      { from: "a", to: "b", kind: "dependency", status: "resolved" },
    ]);
    expect(response.focus_task_id).toBe("b");
    expect(response.nodes[2]).toMatchObject({
      blocked: true,
      blocked_reason: "pending",
      role: "member",
    });
  });

  it("groups subtasks under their parent", () => {
    const { groups } = buildBoardGraph(
      [task("p"), task("x", { parentTaskId: "p" }), task("y", { parentTaskId: "p" }), task("solo")],
      null,
    );
    expect(groups).toEqual([{ id: "group:p", parentId: "p", title: "T p", memberIds: ["x", "y"] }]);
  });

  it("titles a frame from the lookup when the parent is not on the board", () => {
    const { groups } = buildBoardGraph([task("x", { parentTaskId: "gone" })], null, (id) =>
      id === "gone" ? "Off-board parent" : undefined,
    );
    expect(groups[0]).toMatchObject({ parentId: "gone", title: "Off-board parent" });
  });

  it("defaults to a pending edge status", () => {
    const { response } = buildBoardGraph(
      [task("a"), task("b", { dependsOn: [{ id: "a" }] })],
      null,
    );
    expect(response.edges[0].status).toBe("pending");
  });

  it("includes same-board related edges without affecting dependencies", () => {
    const { response } = buildBoardGraph(
      [task("a"), task("b", { related: [{ id: "a" }, { id: "outside" }] })],
      null,
    );
    expect(response.edges).toEqual([{ from: "a", to: "b", kind: "related" }]);
  });

  it("emits one canonical edge for symmetric related references", () => {
    const { response } = buildBoardGraph(
      [task("a", { related: [{ id: "b" }] }), task("b", { related: [{ id: "a" }] })],
      null,
    );

    expect(response.edges).toEqual([{ from: "a", to: "b", kind: "related" }]);
  });
});
