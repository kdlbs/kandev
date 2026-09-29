import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Goal, GoalResponse } from "@/lib/api/domains/coordinator-api";

let goalState: { data: GoalResponse | null; status: string } = { data: null, status: "loading" };
vi.mock("@/hooks/domains/coordinator/use-goal", () => ({
  useGoal: () => ({ ...goalState, reload: vi.fn(), retry: vi.fn() }),
}));

import { GoalNote } from "./goal-note";

const ACTION = "goal-note-action";

function goal(overrides: Partial<Goal> = {}): Goal {
  return {
    id: "g",
    coordinator_id: "c",
    name: "Ship the billing beta",
    due_on: "2999-10-31",
    status: "active",
    criteria: [
      { id: "1", text: "a", done: true },
      { id: "2", text: "b", done: false },
    ],
    baseline: null,
    set_at: "2026-09-20T10:00:00Z",
    met_at: null,
    met_by: null,
    created_at: "2026-09-20T10:00:00Z",
    updated_at: "2026-09-20T10:00:00Z",
    ...overrides,
  } as Goal;
}

function show(response: GoalResponse | null, status = "ready", canManage = true) {
  goalState = { data: response, status };
  render(<GoalNote workspaceId="ws" coordinatorId="c1" canManage={canManage} />);
}

afterEach(() => {
  cleanup();
  goalState = { data: null, status: "loading" };
});

describe("GoalNote", () => {
  it("is hidden while loading and when the goal read fails", () => {
    show(null, "loading");
    expect(screen.queryByTestId("goal-note")).toBeNull();
    cleanup();
    show(null, "error");
    expect(screen.queryByTestId("goal-note")).toBeNull();
  });

  it("offers Set a goal to a manager when no goal was ever set, linking to the Goal section", () => {
    show({ active: null, last_met: null, measures: null });
    expect(screen.getByTestId("goal-note").textContent).toContain(
      "No goal is set, so this list is ordered by urgency alone.",
    );
    expect(screen.getByTestId(ACTION).getAttribute("href")).toBe(
      "/settings/workspaces/ws/coordinators/c1?section=goal",
    );
    expect(screen.getByTestId(ACTION).textContent).toBe("Set a goal");
  });

  it("shows no button to a reader", () => {
    show({ active: null, last_met: null, measures: null }, "ready", false);
    expect(screen.getByTestId("goal-note").textContent).toContain("No goal is set");
    expect(screen.queryByTestId(ACTION)).toBeNull();
  });

  it("shows an active goal with the due date and criteria count", () => {
    show({ active: goal(), last_met: null, measures: null });
    const text = screen.getByTestId("goal-note").textContent ?? "";
    expect(text).toContain("Ship the billing beta");
    expect(text).toContain("Due ");
    expect(text).toContain("2999");
    expect(text).toContain("1 of 2 criteria met");
    expect(screen.queryByTestId(ACTION)).toBeNull();
  });

  it("shows Overdue since for a past due date and omits the date without one", () => {
    show({ active: goal({ due_on: "2020-01-05" }), last_met: null, measures: null });
    expect(screen.getByTestId("goal-note").textContent).toContain("Overdue since ");
    cleanup();
    show({ active: goal({ due_on: null, criteria: [] }), last_met: null, measures: null });
    const text = screen.getByTestId("goal-note").textContent ?? "";
    expect(text).not.toContain("Due ");
    expect(text).not.toContain("Overdue");
    expect(text).toContain("0 of 0 criteria met");
  });

  it("uses the singular for one criterion", () => {
    show({
      active: goal({ criteria: [{ id: "1", text: "a", done: true }] }),
      last_met: null,
      measures: null,
    });
    expect(screen.getByTestId("goal-note").textContent).toContain("1 of 1 criterion met");
  });

  it("shows the last met goal with Set the next goal", () => {
    show({
      active: null,
      last_met: goal({ status: "met", met_at: "2026-09-25T12:00:00Z" }),
      measures: null,
    });
    const text = screen.getByTestId("goal-note").textContent ?? "";
    expect(text).toContain('"Ship the billing beta" was met on ');
    expect(screen.getByTestId(ACTION).textContent).toBe("Set the next goal");
  });
});
