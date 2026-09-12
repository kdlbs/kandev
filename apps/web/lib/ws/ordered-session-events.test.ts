import { describe, expect, it } from "vitest";
import { isRawSessionEvent, orderedCoreDisposition } from "./ordered-session-events";
import type { RawSessionEvent } from "./ordered-session-events";

const validEvent: RawSessionEvent = {
  type: "session.event",
  protocol_version: 1,
  event_type: "message.added",
  session_id: "session-1",
  task_id: "task-1",
  sequence: 1,
  event_id: "event-1",
  payload: { type: "message.added" },
};

describe("ordered session event envelopes", () => {
  it.each([
    ["zero sequence", { sequence: 0 }],
    ["negative sequence", { sequence: -1 }],
    ["fractional sequence", { sequence: 1.5 }],
    ["empty event id", { event_id: "" }],
    ["empty event type", { event_type: "" }],
    ["empty session id", { session_id: "" }],
  ])("rejects %s", (_name, override) => {
    const event = { ...validEvent, ...override };
    expect(isRawSessionEvent(event)).toBe(false);
  });

  it("classifies a structurally valid but incompatible payload as poison", () => {
    expect(
      orderedCoreDisposition({
        ...validEvent,
        payload: { type: "message.updated" },
      }),
    ).toBe("poison");
  });

  it("classifies turn removal as a projectable ordered event", () => {
    expect(
      orderedCoreDisposition({
        ...validEvent,
        event_type: "session.turn.removed",
        payload: {
          type: "session.turn.removed",
          session_id: "session-1",
          task_id: "task-1",
          id: "turn-1",
        },
      }),
    ).toBe("project");
  });
});
