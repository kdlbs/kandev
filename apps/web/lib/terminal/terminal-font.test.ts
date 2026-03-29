import { describe, it, expect } from "vitest";
import { buildTerminalFontFamily, TERMINAL_FONT_PRESETS } from "./terminal-font";

const DEFAULT_FONT = 'Menlo, Monaco, "Courier New", monospace';

describe("buildTerminalFontFamily", () => {
  it("returns default font stack when selection is null", () => {
    expect(buildTerminalFontFamily(null)).toBe(DEFAULT_FONT);
  });

  it("returns default font stack when selection is empty string", () => {
    expect(buildTerminalFontFamily("")).toBe(DEFAULT_FONT);
  });

  it("prepends selected preset font with fallback", () => {
    expect(buildTerminalFontFamily("JetBrains Mono")).toBe(`"JetBrains Mono", ${DEFAULT_FONT}`);
  });

  it("prepends selected nerd font with fallback", () => {
    expect(buildTerminalFontFamily("JetBrainsMono Nerd Font")).toBe(
      `"JetBrainsMono Nerd Font", ${DEFAULT_FONT}`,
    );
  });

  it("handles custom font family string", () => {
    expect(buildTerminalFontFamily("My Custom Font")).toBe(`"My Custom Font", ${DEFAULT_FONT}`);
  });
});

describe("TERMINAL_FONT_PRESETS", () => {
  it("has presets in all three categories", () => {
    const categories = new Set(TERMINAL_FONT_PRESETS.map((p) => p.category));
    expect(categories).toContain("icons");
    expect(categories).toContain("ligatures");
    expect(categories).toContain("system");
  });

  it("each preset has value, label, and category", () => {
    for (const preset of TERMINAL_FONT_PRESETS) {
      expect(preset.value).toBeTruthy();
      expect(preset.label).toBeTruthy();
      expect(preset.category).toBeTruthy();
    }
  });
});
