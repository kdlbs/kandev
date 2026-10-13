import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import type { ActivityItem } from "@/lib/api/domains/coordinator-activity-api";
import { actionText, outcomeLine } from "./activity-text";

const t = ((key: string, opts?: Record<string, string>) =>
  opts ? `${key}:${JSON.stringify(opts)}` : key) as unknown as TFunction;

function item(overrides: Partial<ActivityItem> = {}): ActivityItem {
  return {
    id: "a-1",
    coordinator_id: "c-1",
    workspace_id: "w-1",
    action_class: "create_task",
    outcome: "returned",
    authorization: "requires_approval",
    target_task_id: null,
    proposal_id: "p-1",
    actor_user_id: "u-1",
    reason_code: null,
    detail: "narrower",
    edited: false,
    refusal_count: 0,
    undone_at: null,
    undone_by: null,
    undo_of_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    undoable: false,
    target_task_identifier: null,
    from_step_id: null,
    ...overrides,
  };
}

describe("returned activity rows", () => {
  it("renders the condition detail, or the bare label without one", () => {
    expect(actionText(item(), t)).toBe('coordinator:activityReturnedDetail:{"detail":"narrower"}');
    expect(actionText(item({ detail: "  " }), t)).toBe("coordinator:activityReturned");
  });

  it("names the manager in the authorisation line when known", () => {
    expect(outcomeLine(item(), { kind: "named", name: "Ada" }, t)).toBe(
      'coordinator:activityReturnedBy:{"name":"Ada"}',
    );
    expect(outcomeLine(item(), { kind: "unknown" } as never, t)).toBe(
      "coordinator:activityReturned",
    );
  });
});
