import { describe, expect, it } from "vitest";
import { actorAttributionLabel, usageRowLabel, usageTaskDestination } from "./token-usage-metadata";
import { presentTokenRows } from "./token-usage-utils";
import { retainedRow } from "./token-usage.test-helpers";
const translate = (key: string, options?: Record<string, unknown>) => `${key}:${options?.id ?? ""}`;

describe("trusted metric actor labels", () => {
  it("does not turn an unsupported actor kind into a human user", () => {
    expect(
      actorAttributionLabel(
        { actor_status: "attributed", actor_id: "proven-id", actor_kind: "unknown" },
        translate,
      ),
    ).toBe("common:unknown:");
  });
  it.each([
    ["user", "stats:attributedUser"],
    ["service", "stats:serviceActor"],
    ["synthetic", "stats:syntheticActor"],
  ])("retains the trusted %s actor kind", (kind, label) => {
    expect(
      actorAttributionLabel(
        { actor_status: "attributed", actor_id: "proven-id", actor_kind: kind },
        translate,
      ),
    ).toBe(`${label}:proven-id`);
  });
});

describe("retained usage allocation labels", () => {
  it.each(["tasks", "sessions"] as const)(
    "distinguishes unallocated, deleted, live and archived %s without inventing links",
    (view) => {
      const [unallocated, deleted, live, archived] = presentTokenRows(
        [
          retainedRow({ task_id: "", session_id: "", subject_id: "" }),
          retainedRow(),
          retainedRow({ task_available: true, session_available: true }),
          retainedRow({ task_available: true, session_available: true, task_archived: true }),
        ],
        view,
      );
      expect(usageRowLabel(unallocated, view, translate)).toBe("stats:unallocatedUsage:");
      expect(usageTaskDestination(unallocated, view)).toBeNull();
      expect(usageRowLabel(deleted, view, translate)).toBe(
        view === "tasks" ? "stats:deletedTask:" : "stats:unavailableSession:",
      );
      expect(usageTaskDestination(deleted, view)).toBeNull();
      for (const row of [live, archived]) {
        expect(usageRowLabel(row, view, translate)).toBe(view === "tasks" ? "task-1" : "session-1");
        expect(usageTaskDestination(row, view)).toEqual(
          view === "tasks" ? { taskId: "task-1" } : { taskId: "task-1", sessionId: "session-1" },
        );
      }
    },
  );

  it.each(["tasks", "sessions"] as const)("handles omitted HTTP IDs in %s", (view) => {
    const [row] = presentTokenRows([retainedRow()], view);
    const omitted = { ...row, task_id: undefined, session_id: undefined };
    expect(usageRowLabel(omitted, view, translate)).toBe("stats:unallocatedUsage:");
    expect(usageTaskDestination(omitted, view)).toBeNull();
  });

  it.each(["models", "daily", "monthly"] as const)(
    "keeps %s labels independent of task allocation",
    (view) => {
      const [unallocated, retained] = presentTokenRows(
        [retainedRow({ task_id: "", session_id: "" }), retainedRow()],
        view,
      );
      expect(usageRowLabel(unallocated, view, translate)).toBe(
        usageRowLabel(retained, view, translate),
      );
      expect(usageRowLabel(unallocated, view, translate)).not.toContain("unallocatedUsage");
    },
  );
});
