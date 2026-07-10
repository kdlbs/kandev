import { describe, expect, it } from "vitest";
import { normalizeTerminalFontSize } from "./terminal-settings";

describe("normalizeTerminalFontSize", () => {
  it("clamps finite font sizes to the supported range", () => {
    expect(normalizeTerminalFontSize(6, 13)).toBe(8);
    expect(normalizeTerminalFontSize(18, 13)).toBe(18);
    expect(normalizeTerminalFontSize(30, 13)).toBe(24);
  });

  it("uses the fallback when the input is not finite", () => {
    expect(normalizeTerminalFontSize(Number.NaN, 15)).toBe(15);
    expect(normalizeTerminalFontSize(Number.POSITIVE_INFINITY, 15)).toBe(15);
  });
});
