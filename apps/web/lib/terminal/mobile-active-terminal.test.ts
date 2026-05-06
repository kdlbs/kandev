import { describe, it, expect, vi, beforeEach } from "vitest";
import { setActiveTerminalSender, getActiveTerminalSender } from "./mobile-active-terminal";

describe("mobile active terminal sender registry", () => {
  beforeEach(() => {
    setActiveTerminalSender(null);
  });

  it("returns null when no sender is registered", () => {
    expect(getActiveTerminalSender()).toBeNull();
  });

  it("returns the registered sender", () => {
    const sender = vi.fn();
    setActiveTerminalSender(sender);
    expect(getActiveTerminalSender()).toBe(sender);
    sender("hi");
    expect(sender).toHaveBeenCalledWith("hi");
  });

  it("replaces a previously registered sender", () => {
    const a = vi.fn();
    const b = vi.fn();
    setActiveTerminalSender(a);
    setActiveTerminalSender(b);
    expect(getActiveTerminalSender()).toBe(b);
  });

  it("clears the sender when null is set", () => {
    setActiveTerminalSender(vi.fn());
    setActiveTerminalSender(null);
    expect(getActiveTerminalSender()).toBeNull();
  });
});
