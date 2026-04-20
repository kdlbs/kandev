import { describe, it, expect } from "vitest";
import { classifyTask } from "./task-classify";
import { applySort } from "@/lib/sidebar/apply-view";
import type { TaskSwitcherItem } from "./task-switcher";

const EARLY = "2026-01-01T00:00:00Z";

function task(overrides: Partial<TaskSwitcherItem>): TaskSwitcherItem {
  return { id: "t", title: "t", ...overrides };
}

describe("classifyTask", () => {
  it("buckets WAITING_FOR_INPUT as review", () => {
    expect(classifyTask("WAITING_FOR_INPUT")).toBe("review");
  });

  it("buckets RUNNING as in_progress", () => {
    expect(classifyTask("RUNNING")).toBe("in_progress");
  });

  it("buckets STARTING with REVIEW task state as review", () => {
    expect(classifyTask("STARTING", "REVIEW")).toBe("review");
  });
});

describe("applySort state (regression: silent resume reorder)", () => {
  it("sorts review (turn finished) above in_progress (running)", () => {
    const running = task({ id: "running", sessionState: "RUNNING", createdAt: EARLY });
    const waiting = task({ id: "waiting", sessionState: "WAITING_FOR_INPUT", createdAt: EARLY });
    const sorted = applySort([running, waiting], { key: "state", direction: "asc" });
    expect(sorted.map((t) => t.id)).toEqual(["waiting", "running"]);
  });

  it("orders backlog after both review and in_progress", () => {
    const backlog = task({ id: "bk", sessionState: undefined });
    const running = task({ id: "ru", sessionState: "RUNNING" });
    const review = task({ id: "rv", sessionState: "WAITING_FOR_INPUT" });
    const sorted = applySort([backlog, running, review], { key: "state", direction: "asc" });
    expect(sorted.map((t) => t.id)).toEqual(["rv", "ru", "bk"]);
  });
});
