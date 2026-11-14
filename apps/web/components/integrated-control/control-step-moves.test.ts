import { describe, expect, it } from "vitest";
import type { OverviewEvent } from "@/lib/types/control-overview";
import { historyItems, repeatedMoves } from "./control-step-moves";

const move = (task: string, minute: number, id = `${task}:${minute}`): OverviewEvent => ({
  id,
  kind: "step_move",
  task_id: task,
  title: `Title ${task}`,
  workspace_id: "w",
  at: new Date(Date.UTC(2026, 9, 19, 10, minute)).toISOString(),
});

describe("repeated step moves", () => {
  it("reports the densest 60-minute window once it reaches the threshold", () => {
    const events = [
      ...Array.from({ length: 10 }, (_, i) => move("busy", i * 6)),
      move("busy", 200),
      ...Array.from({ length: 9 }, (_, i) => move("calm", i)),
    ];
    const result = repeatedMoves(events);
    expect([...result.keys()]).toEqual(["busy"]);
    expect(result.get("busy")).toMatchObject({
      count: 10,
      start: move("busy", 0).at,
      end: move("busy", 54).at,
      workspaceId: "w",
      title: "Title busy",
    });
  });
  it("does not count moves spread over more than the window", () => {
    const events = Array.from({ length: 12 }, (_, i) => move("slow", i * 7));
    expect(repeatedMoves(events, 9).get("slow")?.count).toBe(9);
    expect(repeatedMoves(events).has("slow")).toBe(false);
  });
});

describe("history items", () => {
  it("folds a task's consecutive moves into one run placed at its newest move", () => {
    const created: OverviewEvent = { kind: "task_created", task_id: "b", at: move("b", 30).at };
    const newestFirst = [move("a", 40), move("b", 35), move("a", 30), created, move("a", 20)];
    const items = historyItems(newestFirst);
    expect(items.map((item) => item.type)).toEqual(["moves", "event", "event"]);
    const run = items[0];
    expect(run.type === "moves" && run.events.map((event) => event.id)).toEqual([
      "a:40",
      "a:30",
      "a:20",
    ]);
    expect(run.type === "moves" && [run.start, run.end]).toEqual([
      move("a", 20).at,
      move("a", 40).at,
    ]);
  });
  it("breaks a run at another event of the same task or at a long gap", () => {
    const failure: OverviewEvent = { kind: "session_failed", task_id: "a", at: move("a", 25).at };
    const items = historyItems([
      move("a", 50),
      move("a", 30),
      failure,
      move("a", 20),
      move("a", 10),
    ]);
    expect(items.map((item) => (item.type === "moves" ? item.events.length : 1))).toEqual([
      2, 1, 2,
    ]);
    const gap = historyItems([{ ...move("a", 0), at: "2026-10-19T12:30:00.000Z" }, move("a", 0)]);
    expect(gap.map((item) => item.type)).toEqual(["event", "event"]);
  });
});
