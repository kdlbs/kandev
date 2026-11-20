import { describe, expect, it } from "vitest";
import { shouldShowUnreadDot } from "./task-unread-dot-visibility";

const base = {
  enabled: true,
  unreadSessionIds: ["session-a"],
  isSelected: false,
  lingering: false,
  displayedSessionId: null,
};

describe("shouldShowUnreadDot", () => {
  it("shows the dot on an unread task that is not open", () => {
    expect(shouldShowUnreadDot(base)).toBe(true);
    expect(shouldShowUnreadDot({ ...base, unreadSessionIds: undefined })).toBe(false);
  });

  it("hides every dot while the setting is off, even while lingering", () => {
    expect(shouldShowUnreadDot({ ...base, enabled: false, lingering: true })).toBe(false);
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.5
  it("hides the open task's dot when only its displayed session is unread", () => {
    expect(
      shouldShowUnreadDot({ ...base, isSelected: true, displayedSessionId: "session-a" }),
    ).toBe(false);
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.17
  it("keeps the open task's dot while another of its sessions is unread", () => {
    expect(
      shouldShowUnreadDot({
        ...base,
        isSelected: true,
        displayedSessionId: "session-a",
        unreadSessionIds: ["session-a", "session-b"],
      }),
    ).toBe(true);
  });

  it("keeps the dot while the opened task lingers", () => {
    expect(
      shouldShowUnreadDot({
        ...base,
        isSelected: true,
        lingering: true,
        displayedSessionId: "session-a",
        unreadSessionIds: undefined,
      }),
    ).toBe(true);
  });
});
