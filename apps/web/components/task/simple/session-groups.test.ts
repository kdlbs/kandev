import { describe, expect, it } from "vitest";

import { groupSessionsForTimeline } from "./session-groups";
import type { TaskSession } from "@/app/office/tasks/[id]/types";

function session(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: "s1",
    state: "IDLE",
    startedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  } as TaskSession;
}

/**
 * `roleChip` carries a catalog KEY rather than resolved copy: `deriveRoleChip`
 * runs outside a hook scope, so the chip has to re-resolve at render for a
 * locale switch to reach it. These pin the key contract — a regression to a
 * literal ("Approver") would render as a missing key, not as English.
 */
describe("groupSessionsForTimeline — roleChip", () => {
  it("returns the approver key for an agent in the approvers list", () => {
    const groups = groupSessionsForTimeline([session({ agentProfileId: "a1" })], [], ["a1"]);
    expect(groups).toHaveLength(1);
    expect(groups[0].roleChip).toBe("task:roleApprover");
  });

  it("returns the reviewer key for an agent in the reviewers list", () => {
    const groups = groupSessionsForTimeline([session({ agentProfileId: "a1" })], ["a1"], []);
    expect(groups[0].roleChip).toBe("task:roleReviewer");
  });

  it("prefers approver when an agent holds both roles", () => {
    const groups = groupSessionsForTimeline([session({ agentProfileId: "a1" })], ["a1"], ["a1"]);
    expect(groups[0].roleChip).toBe("task:roleApprover");
  });

  it("returns null for an agent in neither list", () => {
    const groups = groupSessionsForTimeline([session({ agentProfileId: "a1" })], ["b2"], ["c3"]);
    expect(groups[0].roleChip).toBeNull();
  });

  it("returns null for a kanban session with no agent profile", () => {
    const groups = groupSessionsForTimeline([session()], ["a1"], ["a1"]);
    expect(groups[0].roleChip).toBeNull();
  });
});
