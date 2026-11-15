import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { hasPendingPermissionRequest } from "./pending-clarification";

function message(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "message",
    created_at: "2026-05-02T00:00:00Z",
    ...overrides,
  };
}

describe("hasPendingPermissionRequest", () => {
  it("detects permission requests with pending status", () => {
    expect(
      hasPendingPermissionRequest([
        message({ type: "permission_request", metadata: { status: "pending" } }),
      ]),
    ).toBe(true);
  });

  it("treats missing permission status as pending", () => {
    expect(hasPendingPermissionRequest([message({ type: "permission_request" })])).toBe(true);
  });

  it("ignores approved permission requests", () => {
    expect(
      hasPendingPermissionRequest([
        message({ type: "permission_request", metadata: { status: "approved" } }),
      ]),
    ).toBe(false);
  });

  it("ignores rejected permission requests", () => {
    expect(
      hasPendingPermissionRequest([
        message({ type: "permission_request", metadata: { status: "rejected" } }),
      ]),
    ).toBe(false);
  });

  it("ignores expired permission requests", () => {
    expect(
      hasPendingPermissionRequest([
        message({ type: "permission_request", metadata: { status: "expired" } }),
      ]),
    ).toBe(false);
  });

  it("returns false for non-permission messages", () => {
    expect(hasPendingPermissionRequest([message({ type: "message" })])).toBe(false);
  });

  it("returns false for empty or null input", () => {
    expect(hasPendingPermissionRequest([])).toBe(false);
    expect(hasPendingPermissionRequest(null)).toBe(false);
    expect(hasPendingPermissionRequest(undefined)).toBe(false);
  });

  // Mixed-state: only the latest permission_request drives the UI. A stale
  // pending row left behind by an earlier crash followed by a newer approved
  // one must not light the amber icon — the agent is no longer blocked on
  // the old row.
  it("returns false when an older permission is still pending but a newer one is approved", () => {
    expect(
      hasPendingPermissionRequest([
        message({ id: "old", type: "permission_request", metadata: { status: "pending" } }),
        message({ id: "new", type: "permission_request", metadata: { status: "approved" } }),
      ]),
    ).toBe(false);
  });

  it("returns true when the latest permission is pending and earlier ones are resolved", () => {
    expect(
      hasPendingPermissionRequest([
        message({ id: "a", type: "permission_request", metadata: { status: "approved" } }),
        message({ id: "b", type: "permission_request", metadata: { status: "rejected" } }),
        message({ id: "c", type: "permission_request", metadata: { status: "pending" } }),
      ]),
    ).toBe(true);
  });

  it("returns false when all permission requests are resolved regardless of order", () => {
    expect(
      hasPendingPermissionRequest([
        message({ id: "a", type: "permission_request", metadata: { status: "approved" } }),
        message({ id: "b", type: "permission_request", metadata: { status: "rejected" } }),
        message({ id: "c", type: "permission_request", metadata: { status: "expired" } }),
      ]),
    ).toBe(false);
  });
});

// Turn-scoped boundary: walking back across a different turn_id ends the
// scan, so a pending row from a previous (crashed) turn must not leak into
// the current turn's indicator.
describe("hasPendingPermissionRequest — turn scoping", () => {
  it("ignores a pending permission from a previous turn", () => {
    expect(
      hasPendingPermissionRequest([
        message({
          id: "old",
          turn_id: "t1",
          type: "permission_request",
          metadata: { status: "pending" },
        }),
        message({ id: "current", turn_id: "t2", type: "message", content: "hi" }),
      ]),
    ).toBe(false);
  });

  it("detects a pending permission in the current turn", () => {
    expect(
      hasPendingPermissionRequest([
        message({
          id: "stale",
          turn_id: "t1",
          type: "permission_request",
          metadata: { status: "pending" },
        }),
        message({
          id: "active",
          turn_id: "t2",
          type: "permission_request",
          metadata: { status: "pending" },
        }),
      ]),
    ).toBe(true);
  });

  it("stops at the turn boundary even when an older turn has a pending permission", () => {
    expect(
      hasPendingPermissionRequest([
        message({
          id: "old-pending",
          turn_id: "t1",
          type: "permission_request",
          metadata: { status: "pending" },
        }),
        message({
          id: "new-approved",
          turn_id: "t2",
          type: "permission_request",
          metadata: { status: "approved" },
        }),
      ]),
    ).toBe(false);
  });

  it("ignores a legacy null-turn_id pending permission when a turn is active", () => {
    expect(
      hasPendingPermissionRequest([
        message({ id: "legacy", type: "permission_request", metadata: { status: "pending" } }),
        message({ id: "current", turn_id: "t2", type: "message", content: "hi" }),
      ]),
    ).toBe(false);
  });
});
