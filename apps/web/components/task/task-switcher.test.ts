import { describe, it, expect } from "vitest";
import { classifyTask } from "./task-classify";
import { statePriority, sortByStateThenCreated, type TaskSwitcherItem } from "./task-switcher";

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

describe("sortByStateThenCreated (regression: silent resume reorder)", () => {
  it("sorts review (turn finished) above in_progress (running)", () => {
    // Regression: when a task is auto-resumed after backend restart, its
    // session transiently cycles WAITING_FOR_INPUT -> STARTING -> RUNNING ->
    // WAITING_FOR_INPUT. The sidebar must NOT promote it above other
    // turn-finished tasks during the RUNNING flicker.
    const running = task({
      id: "running",
      sessionState: "RUNNING",
      createdAt: EARLY,
    });
    const waiting = task({
      id: "waiting",
      sessionState: "WAITING_FOR_INPUT",
      createdAt: EARLY,
    });

    expect(statePriority(waiting)).toBeLessThan(statePriority(running));
    expect([running, waiting].sort(sortByStateThenCreated).map((t) => t.id)).toEqual([
      "waiting",
      "running",
    ]);
  });

  it("does not move a transiently-RUNNING task above older turn-finished tasks", () => {
    // Older turn-finished task A and a newer turn-finished task B that gets
    // resumed (so its sessionState briefly flips to RUNNING). With the
    // pre-fix priority (in_progress=0), B would jump to the top. With the
    // fix (review=0), B stays in its original position relative to A.
    const a = task({
      id: "a-old-waiting",
      sessionState: "WAITING_FOR_INPUT",
      createdAt: EARLY,
    });
    const b = task({
      id: "b-newer-resuming",
      sessionState: "RUNNING",
      createdAt: "2026-01-02T00:00:00Z",
    });

    const sorted = [a, b].sort(sortByStateThenCreated).map((t) => t.id);
    expect(sorted[0]).toBe("a-old-waiting");
    expect(sorted[1]).toBe("b-newer-resuming");
  });

  it("orders backlog after both review and in_progress", () => {
    const backlog = task({ id: "bk", sessionState: undefined });
    const running = task({ id: "ru", sessionState: "RUNNING" });
    const review = task({ id: "rv", sessionState: "WAITING_FOR_INPUT" });

    expect([backlog, running, review].sort(sortByStateThenCreated).map((t) => t.id)).toEqual([
      "rv",
      "ru",
      "bk",
    ]);
  });

  it("breaks ties within a bucket by createdAt descending", () => {
    const older = task({
      id: "older",
      sessionState: "WAITING_FOR_INPUT",
      createdAt: EARLY,
    });
    const newer = task({
      id: "newer",
      sessionState: "WAITING_FOR_INPUT",
      createdAt: "2026-02-01T00:00:00Z",
    });

    expect([older, newer].sort(sortByStateThenCreated).map((t) => t.id)).toEqual([
      "newer",
      "older",
    ]);
  });
});
