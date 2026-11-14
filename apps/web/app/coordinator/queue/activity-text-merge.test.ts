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
    action_class: "merge",
    outcome: "uncertain",
    authorization: "requires_approval",
    target_task_id: null,
    proposal_id: "p-1",
    actor_user_id: "u-1",
    reason_code: null,
    detail: "",
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

const p = { name: null } as never;

describe("merge activity rows", () => {
  it("labels an uncertain merge with and without detail", () => {
    expect(actionText(item(), t)).toBe("coordinator:activityUncertain");
    expect(actionText(item({ detail: "no answer" }), t)).toBe(
      'coordinator:activityUncertainDetail:{"detail":"no answer"}',
    );
    expect(outcomeLine(item(), p, t)).toBe("coordinator:activityUncertain");
  });

  it("explains a refused merge by its refusal code", () => {
    expect(actionText(item({ outcome: "refused", reason_code: "stale_head" }), t)).toBe(
      'coordinator:activityRefusedCode:{"code":"coordinator:mergeReasonStaleHead"}',
    );
  });
});
