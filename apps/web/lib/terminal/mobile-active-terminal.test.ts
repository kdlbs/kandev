import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  getActiveTerminalSender,
  getActiveTerminalTarget,
  setActiveTerminalSender,
} from "./mobile-active-terminal";

beforeEach(() => setActiveTerminalSender(null));

describe("mobile terminal ownership", () => {
  it("does not send keys to another session", () => {
    const sender = vi.fn();
    setActiveTerminalSender(sender, "first-session");
    expect(getActiveTerminalSender("second-session")).toBeNull();
    expect(getActiveTerminalSender("first-session")).toBe(sender);
  });

  it("keeps a successor sender when an old terminal releases ownership", () => {
    const release = setActiveTerminalSender(vi.fn(), "session", () => false);
    const successor = vi.fn();
    setActiveTerminalSender(successor, "session", () => true);
    expect(typeof release).toBe("function");
    release();
    expect(getActiveTerminalSender("session")).toBe(successor);
    expect(getActiveTerminalTarget("session")?.applicationCursorMode?.()).toBe(true);
    expect(getActiveTerminalTarget("another-session")).toBeNull();
  });
});
