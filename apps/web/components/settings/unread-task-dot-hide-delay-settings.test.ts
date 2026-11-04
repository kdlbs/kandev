import { describe, expect, it } from "vitest";
import { parseUnreadTaskDotHideDelay } from "./unread-task-dot-hide-delay-settings";

describe("parseUnreadTaskDotHideDelay", () => {
  it("accepts whole seconds from 0 to 60", () => {
    expect(parseUnreadTaskDotHideDelay("0")).toBe(0);
    expect(parseUnreadTaskDotHideDelay("5")).toBe(5);
    expect(parseUnreadTaskDotHideDelay("60")).toBe(60);
  });

  it("rejects values the backend would reject", () => {
    for (const value of ["", "61", "-1", "2.5", "abc"]) {
      expect(parseUnreadTaskDotHideDelay(value)).toBeNull();
    }
  });
});
