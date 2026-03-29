import { describe, it, expect } from "vitest";
import { buildTerminalFontFamily, TERMINAL_FONT_PRESETS, DEFAULT_FONT_SIZE } from "./terminal-font";

const DEFAULT_FONT = 'Menlo, Monaco, "Courier New", monospace';

describe("buildTerminalFontFamily", () => {
  it("returns default font stack when selection is null", () => {
    expect(buildTerminalFontFamily(null)).toBe(DEFAULT_FONT);
  });

  it("returns default font stack when selection is empty string", () => {
    expect(buildTerminalFontFamily("")).toBe(DEFAULT_FONT);
  });

  it("returns preset value as-is without wrapping", () => {
    const preset = '"JetBrains Mono", "Fira Code", Menlo, Consolas, monospace';
    expect(buildTerminalFontFamily(preset)).toBe(preset);
  });

  it("returns custom string as-is without wrapping", () => {
    const custom = "My Custom Font, monospace";
    expect(buildTerminalFontFamily(custom)).toBe(custom);
  });
});

describe("TERMINAL_FONT_PRESETS", () => {
  it("has presets in all three categories", () => {
    const categories = new Set(TERMINAL_FONT_PRESETS.map((p) => p.category));
    expect(categories).toContain("icons");
    expect(categories).toContain("ligatures");
    expect(categories).toContain("system");
  });

  it("each preset value contains monospace as a fallback", () => {
    for (const preset of TERMINAL_FONT_PRESETS) {
      expect(preset.value).toContain("monospace");
    }
  });

  it("each preset has value, label, and category", () => {
    for (const preset of TERMINAL_FONT_PRESETS) {
      expect(preset.value).toBeTruthy();
      expect(preset.label).toBeTruthy();
      expect(preset.category).toBeTruthy();
    }
  });
});

describe("DEFAULT_FONT_SIZE", () => {
  it("equals 13", () => {
    expect(DEFAULT_FONT_SIZE).toBe(13);
  });
});
