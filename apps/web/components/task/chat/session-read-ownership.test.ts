import { describe, expect, it } from "vitest";
import { isSessionReadableInPanel } from "./session-read-ownership";

describe("isSessionReadableInPanel", () => {
  it("does not read another task's session left in a visible panel", () => {
    expect(isSessionReadableInPanel(true, "task-sleep", "task-other")).toBe(false);
  });

  it("does not read a session whose owner is not loaded yet", () => {
    expect(isSessionReadableInPanel(true, undefined, "task-other")).toBe(false);
  });

  it("reads the shown task's own session only while visible", () => {
    expect(isSessionReadableInPanel(true, "task-a", "task-a")).toBe(true);
    expect(isSessionReadableInPanel(false, "task-a", "task-a")).toBe(false);
  });

  it("reads in hosts that are not bound to a task", () => {
    expect(isSessionReadableInPanel(true, "task-a", null)).toBe(true);
  });
});
