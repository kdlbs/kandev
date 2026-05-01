import { describe, it, expect } from "vitest";
import { applyShellModifiers } from "./apply-shell-modifiers";

const OFF = { ctrl: false, shift: false };

describe("applyShellModifiers", () => {
  it("passes data through when no modifier is active", () => {
    expect(applyShellModifiers("c", OFF)).toBe("c");
    expect(applyShellModifiers("\t", OFF)).toBe("\t");
    expect(applyShellModifiers("hello", OFF)).toBe("hello");
  });

  it("maps ctrl + lowercase letter to control byte", () => {
    expect(applyShellModifiers("c", { ctrl: true, shift: false })).toBe("\x03");
    expect(applyShellModifiers("a", { ctrl: true, shift: false })).toBe("\x01");
    expect(applyShellModifiers("z", { ctrl: true, shift: false })).toBe("\x1a");
  });

  it("maps ctrl + uppercase letter to control byte", () => {
    expect(applyShellModifiers("C", { ctrl: true, shift: false })).toBe("\x03");
    expect(applyShellModifiers("Z", { ctrl: true, shift: false })).toBe("\x1a");
  });

  it("passes ctrl + non-letter through unchanged", () => {
    expect(applyShellModifiers("1", { ctrl: true, shift: false })).toBe("1");
    expect(applyShellModifiers("\t", { ctrl: true, shift: false })).toBe("\t");
  });

  it("does not transform multi-char input even when ctrl is active", () => {
    expect(applyShellModifiers("hello", { ctrl: true, shift: false })).toBe("hello");
  });

  it("maps shift + tab to CSI Z (reverse tab)", () => {
    expect(applyShellModifiers("\t", { ctrl: false, shift: true })).toBe("\x1b[Z");
  });

  it("passes shift + non-tab letters through unchanged", () => {
    expect(applyShellModifiers("c", { ctrl: false, shift: true })).toBe("c");
  });

  it("ctrl takes precedence over shift for letters", () => {
    expect(applyShellModifiers("c", { ctrl: true, shift: true })).toBe("\x03");
  });
});
