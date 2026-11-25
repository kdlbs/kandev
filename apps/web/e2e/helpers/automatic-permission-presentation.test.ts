import { describe, expect, it } from "vitest";
import { PermissionFrameGate } from "./automatic-permission-presentation";

function permissionAdded(messageId: string) {
  return {
    action: "session.message.added",
    payload: { message_id: messageId, type: "permission_request" },
  };
}

function permissionUpdated(messageId: string) {
  return {
    action: "session.message.updated",
    payload: { message_id: messageId, type: "permission_request" },
  };
}

function orderedPermissionEvent(kind: "added" | "updated", messageId: string) {
  return {
    type: "session.event",
    event_type: `message.${kind}`,
    payload: { message_id: messageId, message_type: "permission_request" },
  };
}

describe("PermissionFrameGate", () => {
  it("holds only completion updates for the observed permission candidate", () => {
    const gate = new PermissionFrameGate();
    expect(gate.observe(permissionUpdated("permission-1"))).toBe("pass");
    expect(gate.observe(permissionAdded("permission-1"))).toBe("pass");
    expect(gate.observe(permissionUpdated("permission-2"))).toBe("pass");
    expect(gate.observe(permissionUpdated("permission-1"))).toBe("hold");
  });

  it("passes unrelated message traffic", () => {
    const gate = new PermissionFrameGate();
    expect(
      gate.observe({
        action: "session.message.updated",
        payload: { message_id: "tool-1", type: "tool_call" },
      }),
    ).toBe("pass");
    expect(
      gate.observe({
        action: "session.state_changed",
        payload: { message_id: "permission-1", type: "permission_request" },
      }),
    ).toBe("pass");
  });

  it("matches ordered session.event frames for the observed candidate", () => {
    const gate = new PermissionFrameGate();
    expect(gate.observe(orderedPermissionEvent("added", "permission-1"))).toBe("pass");
    expect(gate.observe(orderedPermissionEvent("updated", "permission-1"))).toBe("hold");
    expect(gate.observe(orderedPermissionEvent("updated", "permission-2"))).toBe("pass");
  });

  it("releases every held frame once, on success and after a failure", () => {
    const gate = new PermissionFrameGate();
    gate.observe(permissionAdded("permission-1"));
    gate.observe(permissionUpdated("permission-1"));
    gate.hold("frame-1");
    gate.hold("frame-2");

    expect(gate.heldCount()).toBe(2);
    expect(gate.release()).toEqual(["frame-1", "frame-2"]);
    expect(gate.heldCount()).toBe(0);
    expect(gate.release()).toEqual([]);
  });

  it("scopes permission and snapshot matching to the target session", () => {
    const gate = new PermissionFrameGate("session-target");
    expect(
      gate.observe({
        action: "session.message.added",
        payload: {
          session_id: "session-other",
          message_id: "permission-other",
          type: "permission_request",
        },
      }),
    ).toBe("pass");
    expect(
      gate.observe({
        action: "session.message.updated",
        payload: {
          session_id: "session-other",
          message_id: "permission-other",
          type: "permission_request",
        },
      }),
    ).toBe("pass");

    gate.observeClientFrame({
      type: "request",
      action: "message.list",
      id: "request-other",
      payload: { session_id: "session-other" },
    });
    expect(gate.shouldHoldResponse({ type: "response", id: "request-other" })).toBe(false);

    gate.observeClientFrame({
      type: "request",
      action: "message.list",
      id: "request-target",
      payload: { session_id: "session-target" },
    });
    expect(gate.shouldHoldResponse({ type: "response", id: "request-target" })).toBe(true);
    expect(gate.shouldHoldResponse({ type: "error", id: "request-target" })).toBe(true);
    expect(gate.shouldHoldResponse({ type: "response", id: "unrelated" })).toBe(false);
  });
});
