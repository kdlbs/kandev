import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import type { ActivityItem } from "@/lib/api/domains/coordinator-activity-api";
import { authorizationLine, outcomeLine } from "./activity-text";

const t = ((key: string, opts?: Record<string, string>) =>
  opts ? `${key}:${JSON.stringify(opts)}` : key) as unknown as TFunction;

const row = (over: Partial<ActivityItem> = {}): ActivityItem =>
  ({
    id: "a-1",
    coordinator_id: "c-1",
    workspace_id: "w-1",
    action_class: "create_task",
    outcome: "approved",
    authorization: "automatic",
    actor_user_id: "u-1",
    detail: null,
    edited: false,
    ...over,
  }) as ActivityItem;

describe("automatic activity rows", () => {
  it("labels the authorization as automatic", () => {
    expect(authorizationLine(row(), t)).toBe("coordinator:activityAuthAutomatic");
  });

  it("names the raising manager on an automatic approval, or omits the name", () => {
    expect(outcomeLine(row(), { kind: "named", name: "Ada" }, t)).toBe(
      'coordinator:activityApprovedAutomatically:{"name":"Ada"}',
    );
    expect(outcomeLine(row(), { kind: "unknown" } as never, t)).toBe(
      "coordinator:activityApprovedAutomaticallyNoName",
    );
  });

  it("does not use the automatic wording for a manager's approval", () => {
    expect(
      outcomeLine(row({ authorization: "requires_approval" }), { kind: "named", name: "Ada" }, t),
    ).not.toContain("Automatically");
  });
});
