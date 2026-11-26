import { describe, expect, it } from "vitest";
import { buildSessionRows, visibleMobileSessionRows } from "./mobile-session-rows";
import type { TaskSession } from "@/lib/types/http";

const session = (id: string, overrides: Partial<TaskSession> = {}) =>
  ({
    id,
    task_id: "task",
    state: "WAITING_FOR_INPUT",
    started_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  }) as TaskSession;

describe("mobile session history rows", () => {
  it("retains the real primary when a task-level primary ID is stale", () => {
    const rows = buildSessionRows(
      [
        session("primary", { is_primary: true, archived_at: "2026-01-02T00:00:00Z" }),
        session("helper", { state: "COMPLETED" }),
      ],
      [],
      "missing",
      new Map(),
    );
    expect(rows[0].isPrimary).toBe(true);
    expect(visibleMobileSessionRows(rows, false, null).map((row) => row.id)).toEqual(["primary"]);
  });

  it("uses a retained profile snapshot label and visible row indices", () => {
    const rows = buildSessionRows(
      [
        session("primary", { is_primary: true }),
        session("hidden", { state: "COMPLETED" }),
        session("visible", { agent_profile_snapshot: { label: "Snapshot Agent" } }),
      ],
      [],
      "primary",
      new Map(),
    );
    const visible = visibleMobileSessionRows(rows, false, "primary");
    expect(visible.map((row) => [row.id, row.index])).toEqual([
      ["primary", 1],
      ["visible", 2],
    ]);
    expect(visible[1].agentLabel).toBe("Snapshot Agent");
  });
});
