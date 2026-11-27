import { describe, expect, it } from "vitest";
import type { Goal } from "@/lib/api/domains/coordinator-api";
import {
  buildPutGoalRequest,
  EMPTY_GOAL_FORM,
  formatCalendarDate,
  formatTimestampDate,
  goalFormFromGoal,
  isGoalFormDirty,
  isOverdue,
  metCriteriaCount,
  withServerDoneStates,
} from "./goal-form";

const DUE = "2026-10-31";

const goal: Goal = {
  id: "g-1",
  coordinator_id: "c-1",
  name: "Ship the billing beta",
  due_on: DUE,
  status: "active",
  criteria: [
    { id: "k1", text: "Invoices render", done: true },
    { id: "k2", text: "Webhooks retried", done: false },
  ],
  baseline: null,
  set_at: "2026-09-20T12:00:00Z",
  met_at: null,
  met_by: null,
  created_at: "2026-09-20T12:00:00Z",
  updated_at: "2026-09-20T12:00:00Z",
};

describe("goal form", () => {
  it("loads a goal into a form and is clean", () => {
    const form = goalFormFromGoal(goal);
    expect(form.name).toBe("Ship the billing beta");
    expect(form.dueOn).toBe(DUE);
    expect(form.criteria.map((c) => c.id)).toEqual(["k1", "k2"]);
    expect(isGoalFormDirty(form, form)).toBe(false);
  });

  it("does not count a done change as dirty but counts text, order, add and remove", () => {
    const saved = goalFormFromGoal(goal);
    const toggled = {
      ...saved,
      criteria: saved.criteria.map((c) => ({ ...c, done: !c.done })),
    };
    expect(isGoalFormDirty(toggled, saved)).toBe(false);
    expect(isGoalFormDirty({ ...saved, name: "x" }, saved)).toBe(true);
    expect(isGoalFormDirty({ ...saved, dueOn: "" }, saved)).toBe(true);
    expect(isGoalFormDirty({ ...saved, criteria: [...saved.criteria].reverse() }, saved)).toBe(
      true,
    );
    expect(isGoalFormDirty({ ...saved, criteria: saved.criteria.slice(1) }, saved)).toBe(true);
    expect(
      isGoalFormDirty(
        { ...saved, criteria: [...saved.criteria, { key: "n", text: "New", done: false }] },
        saved,
      ),
    ).toBe(true);
  });

  it("builds the PUT body with goal_id, trimmed text, ids only for saved criteria", () => {
    const saved = goalFormFromGoal(goal);
    const form = {
      ...saved,
      name: "  Renamed ",
      dueOn: "",
      criteria: [...saved.criteria, { key: "n", text: " New one ", done: false }],
    };
    expect(buildPutGoalRequest(form, "g-1")).toEqual({
      goal_id: "g-1",
      name: "Renamed",
      due_on: null,
      criteria: [
        { id: "k1", text: "Invoices render" },
        { id: "k2", text: "Webhooks retried" },
        { text: "New one" },
      ],
    });
  });

  it("omits goal_id for the empty form and clears a due date with null", () => {
    const req = buildPutGoalRequest({ ...EMPTY_GOAL_FORM, name: "A" }, undefined);
    expect(req).toEqual({ name: "A", due_on: null, criteria: [] });
    expect("goal_id" in req).toBe(false);
  });

  it("takes only done states from a refetched goal and keeps unsaved edits", () => {
    const saved = goalFormFromGoal(goal);
    const draft = {
      ...saved,
      name: "Edited",
      criteria: [
        { ...saved.criteria[0], text: "Edited text" },
        saved.criteria[1],
        { key: "n", text: "New", done: false },
      ],
    };
    const server: Goal = {
      ...goal,
      name: "Server name",
      criteria: [
        { id: "k1", text: "Invoices render", done: false },
        { id: "k2", text: "Webhooks retried", done: true },
      ],
    };
    const next = withServerDoneStates(draft, server);
    expect(next.name).toBe("Edited");
    expect(next.criteria[0]).toMatchObject({ text: "Edited text", done: false });
    expect(next.criteria[1].done).toBe(true);
    expect(next.criteria[2]).toMatchObject({ key: "n", done: false });
  });

  it("counts met criteria", () => {
    expect(metCriteriaCount(goal.criteria)).toBe(1);
  });
});

describe("goal dates", () => {
  it("compares the due date with today in the viewer's zone, not UTC", () => {
    // 2026-10-31 23:30 local is still 31 Oct locally even where UTC is 1 Nov.
    const lateLocal = new Date(2026, 9, 31, 23, 30);
    expect(isOverdue(DUE, lateLocal)).toBe(false);
    expect(isOverdue("2026-10-30", lateLocal)).toBe(true);
    const earlyLocal = new Date(2026, 10, 1, 0, 30);
    expect(isOverdue(DUE, earlyLocal)).toBe(true);
    expect(isOverdue(null, earlyLocal)).toBe(false);
  });

  it("shows a calendar date as stored, with the year, in any zone", () => {
    expect(formatCalendarDate(DUE, "en-GB")).toBe("31 Oct 2026");
    expect(formatCalendarDate("2026-01-01", "en-GB")).toBe("1 Jan 2026");
  });

  it("shows a timestamp as a local-zone date with the year", () => {
    const ts = new Date(2026, 8, 20, 20, 0).toISOString();
    expect(formatTimestampDate(ts, "en-GB")).toMatch(/^20 Sep/);
    expect(formatTimestampDate(ts, "en-GB")).toContain("2026");
  });
});
