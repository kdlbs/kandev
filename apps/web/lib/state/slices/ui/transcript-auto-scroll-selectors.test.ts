import { describe, it, expect } from "vitest";
import { isTranscriptAutoScrollEnabled } from "./transcript-auto-scroll-selectors";

describe("isTranscriptAutoScrollEnabled", () => {
  it("defaults to enabled for a session with no recorded preference", () => {
    expect(isTranscriptAutoScrollEnabled({}, "session-a")).toBe(true);
  });

  it("defaults to enabled when sessionId is null", () => {
    expect(isTranscriptAutoScrollEnabled({ "session-a": false }, null)).toBe(true);
  });

  it("returns the recorded false preference for the session", () => {
    expect(isTranscriptAutoScrollEnabled({ "session-a": false }, "session-a")).toBe(false);
  });

  it("returns the recorded true preference for the session", () => {
    expect(
      isTranscriptAutoScrollEnabled({ "session-a": false, "session-b": true }, "session-b"),
    ).toBe(true);
  });

  it("does not leak one session's preference into another", () => {
    expect(isTranscriptAutoScrollEnabled({ "session-a": false }, "session-b")).toBe(true);
  });
});
