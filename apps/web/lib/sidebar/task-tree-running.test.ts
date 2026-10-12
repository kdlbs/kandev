import { describe, expect, it } from "vitest";
import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import { resolveTaskTreeRunning } from "./task-tree-running";

const task = (value: Partial<TaskSwitcherItem> & Pick<TaskSwitcherItem, "id">) =>
  ({ state: "TODO", ...value }) as TaskSwitcherItem;

describe("task tree running projection", () => {
  it("promotes strict primary-session RUNNING descendants through included ancestors", () => {
    const parent = task({ id: "parent", sessionState: "IDLE" });
    const child = task({ id: "child", parentTaskId: "parent", sessionState: "RUNNING" });
    const workflowOnly = task({ id: "workflow", state: "IN_PROGRESS", sessionState: "IDLE" });
    const children = new Map([["parent", [child]]]);

    expect(resolveTaskTreeRunning([parent, child, workflowOnly], children)).toEqual(
      new Map([
        ["parent", true],
        ["child", true],
        ["workflow", false],
      ]),
    );
  });

  it("ignores excluded descendants and terminates on a corrupt cycle", () => {
    const parent = task({ id: "parent", sessionState: "IDLE" });
    const excluded = task({ id: "excluded", parentTaskId: "parent", sessionState: "RUNNING" });
    const children = new Map([
      ["parent", [excluded]],
      ["excluded", [parent]],
    ]);

    expect(resolveTaskTreeRunning([parent], children)).toEqual(new Map([["parent", false]]));
  });

  it("uses an explicit task-wide aggregate and keeps the legacy primary fallback", () => {
    const taskWide = task({
      id: "task-wide",
      sessionState: "WAITING_FOR_INPUT",
      hasRunningSession: true,
    });
    const explicitFalse = task({
      id: "explicit-false",
      sessionState: "RUNNING",
      hasRunningSession: false,
    });
    const legacy = task({ id: "legacy", sessionState: "RUNNING" });

    expect(resolveTaskTreeRunning([taskWide, explicitFalse, legacy], new Map())).toEqual(
      new Map([
        ["task-wide", true],
        ["explicit-false", false],
        ["legacy", true],
      ]),
    );
  });
});
