import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { FontCategory } from "@/lib/terminal/terminal-font";
import { TERMINAL_FONT_PRESETS } from "@/lib/terminal/terminal-font";
import {
  CATEGORY_BADGE_KEYS,
  CATEGORY_LABEL_KEYS,
  normalizeTerminalFontSize,
} from "./terminal-settings";

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

describe("font category labels", () => {
  const categories: FontCategory[] = ["icons", "ligatures", "system"];

  it("holds catalog keys rather than copy", () => {
    // The tables are module scope, so a `t()` call there would freeze at the
    // boot locale. Keys must survive to render time — see docs/i18n.md.
    for (const key of [
      ...Object.values(CATEGORY_LABEL_KEYS),
      ...Object.values(CATEGORY_BADGE_KEYS),
    ])
      expect(key).toMatch(/^(settings|common):[a-zA-Z]+$/);
  });

  it("resolves every category the presets use", () => {
    for (const category of categories) {
      const label = t(CATEGORY_LABEL_KEYS[category]);
      expect(label).not.toBe(CATEGORY_LABEL_KEYS[category]);
      expect(label.length).toBeGreaterThan(0);
    }
    expect(t(CATEGORY_LABEL_KEYS.icons)).toBe("Nerd Fonts");
    expect(t(CATEGORY_LABEL_KEYS.system)).toBe("System");
    expect(t(CATEGORY_BADGE_KEYS.ligatures as string)).toBe("Ligatures");

    const used = new Set(TERMINAL_FONT_PRESETS.map((preset) => preset.category));
    for (const category of used) expect(CATEGORY_LABEL_KEYS[category]).toBeTruthy();
  });
});
