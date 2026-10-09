import { describe, expect, it } from "vitest";
import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import { applySort } from "./apply-view";
import { normalizeSidebarSort, sidebarSortRules } from "./sidebar-sort-chain";

const task = (value: Partial<TaskSwitcherItem> & Pick<TaskSwitcherItem, "id">) =>
  ({ title: value.id, ...value }) as TaskSwitcherItem;

describe("sidebar sort chain normalization", () => {
  it("keeps valid criteria in order and drops duplicates or malformed values", () => {
    const normalized = normalizeSidebarSort({
      key: "running",
      direction: "desc",
      thenBy: [
        { key: "title", direction: "asc" },
        { key: "title", direction: "desc" },
        { key: "lastActivityAt", direction: "sideways" },
      ],
    });

    expect(normalized.sort).toEqual({
      key: "running",
      direction: "desc",
      thenBy: [{ key: "title", direction: "asc" }],
    });
    expect(normalized.droppedRuleCount).toBe(2);
  });

  it("expands the partial implementation's legacy composite sort", () => {
    expect(normalizeSidebarSort({ key: "runningFirstActivity", direction: "asc" }).sort).toEqual({
      key: "running",
      direction: "desc",
      thenBy: [{ key: "lastActivityAt", direction: "desc" }],
    });
  });

  it("keeps custom order standalone and caps chains at ten rules", () => {
    const custom = normalizeSidebarSort({
      key: "custom",
      direction: "desc",
      thenBy: [{ key: "title", direction: "asc" }],
    });
    expect(custom.sort).toEqual({ key: "title", direction: "asc" });
    expect(custom.droppedRuleCount).toBe(1);

    const long = normalizeSidebarSort({
      key: "state",
      direction: "asc",
      thenBy: Array.from({ length: 12 }, (_, index) => ({
        key: ["updatedAt", "createdAt", "title", "lastActivityAt", "running", "color"][index % 6],
        direction: "desc",
        color: index % 6 === 5 ? `color-${index}` : undefined,
      })),
    });
    expect(sidebarSortRules(long.sort)).toHaveLength(6);
    expect(long.droppedRuleCount).toBe(7);
  });

  it("retains distinct preferred colors but rejects a repeated token", () => {
    const normalized = normalizeSidebarSort({
      key: "color",
      color: "red",
      direction: "desc",
      thenBy: [
        { key: "color", color: "blue", direction: "asc" },
        { key: "color", color: "red", direction: "asc" },
      ],
    });

    expect(sidebarSortRules(normalized.sort)).toEqual([
      { key: "color", color: "red", direction: "desc" },
      { key: "color", color: "blue", direction: "asc" },
    ]);
    expect(normalized.droppedRuleCount).toBe(1);
  });
});

describe("sidebar sort chain evaluation", () => {
  it("compares each rule in order and aggregates strict running before activity", () => {
    const tasks = [
      task({
        id: "waiting-new",
        sessionState: "WAITING_FOR_INPUT",
        lastActivityAt: "2026-10-04T00:00:00Z",
      }),
      task({ id: "running-old", sessionState: "RUNNING", lastActivityAt: "2026-10-02T00:00:00Z" }),
      task({
        id: "workflow-only",
        state: "IN_PROGRESS",
        sessionState: "IDLE",
        lastActivityAt: "2026-10-05T00:00:00Z",
      }),
      task({ id: "running-new", sessionState: "RUNNING", lastActivityAt: "2026-10-03T00:00:00Z" }),
      task({
        id: "waiting-old",
        sessionState: "WAITING_FOR_INPUT",
        lastActivityAt: "2026-10-01T00:00:00Z",
      }),
    ];

    expect(
      applySort(tasks, {
        key: "running",
        direction: "desc",
        thenBy: [{ key: "lastActivityAt", direction: "desc" }],
      }).map(({ id }) => id),
    ).toEqual(["running-new", "running-old", "workflow-only", "waiting-new", "waiting-old"]);
  });

  it("uses descendant runtime and activity projections for a root criterion", () => {
    const parent = task({
      id: "parent",
      sessionState: "IDLE",
      lastActivityAt: "2026-10-01T00:00:00Z",
    });
    const child = task({
      id: "child",
      parentTaskId: "parent",
      sessionState: "RUNNING",
      lastActivityAt: "2026-10-04T00:00:00Z",
    });
    const peer = task({
      id: "peer",
      sessionState: "WAITING_FOR_INPUT",
      lastActivityAt: "2026-10-05T00:00:00Z",
    });

    expect(
      applySort(
        [peer, parent, child],
        {
          key: "running",
          direction: "desc",
          thenBy: [{ key: "lastActivityAt", direction: "desc" }],
        },
        [],
        new Map([["parent", [child]]]),
      ).map(({ id }) => id),
    ).toEqual(["parent", "child", "peer"]);
  });
});
