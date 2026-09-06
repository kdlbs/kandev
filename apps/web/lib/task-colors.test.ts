import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  clearLegacyTaskColors,
  readLegacyTaskColors,
  TASK_COLORS,
  TASK_COLOR_BAR_CLASS,
  TASK_COLOR_LABEL_KEYS,
  TASK_COLORS_STORAGE_KEY,
  parseSidebarTaskColors,
} from "./task-colors";
import { i18n, t } from "@/lib/i18n";

describe("legacy task colors migration input", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("reads valid legacy colors without treating them as server values", () => {
    window.localStorage.setItem(
      TASK_COLORS_STORAGE_KEY,
      JSON.stringify({
        "task-1": "red",
        "task-cleared": null,
        "task-automatic-gray": "gray",
        "task-2": "blue",
      }),
    );
    expect(readLegacyTaskColors()).toEqual({ "task-1": "red", "task-2": "blue" });
  });

  it("returns no values for malformed legacy storage", () => {
    window.localStorage.setItem(TASK_COLORS_STORAGE_KEY, "{not json");
    expect(readLegacyTaskColors()).toEqual({});
  });

  it("removes the legacy key only when migration asks for removal", () => {
    window.localStorage.setItem(TASK_COLORS_STORAGE_KEY, JSON.stringify({ "task-1": "red" }));
    clearLegacyTaskColors();
    expect(window.localStorage.getItem(TASK_COLORS_STORAGE_KEY)).toBeNull();
  });
});

describe("TASK_COLOR_LABEL_KEYS", () => {
  afterEach(async () => {
    await i18n.changeLanguage("en");
  });

  it("covers every colour, keyed by the persisted wire value", () => {
    expect(Object.keys(TASK_COLOR_LABEL_KEYS).sort()).toEqual([...TASK_COLORS].sort());
    // The same wire values index the class table, so they are identity.
    expect(Object.keys(TASK_COLOR_BAR_CLASS).sort()).toEqual([...TASK_COLORS].sort());
  });

  it("holds catalog keys that resolve to English copy", () => {
    expect(TASK_COLOR_LABEL_KEYS.red).toBe("task:colorRed");
    const labels = TASK_COLORS.map((c) => t(TASK_COLOR_LABEL_KEYS[c]));
    expect(labels).toEqual(["Red", "Orange", "Yellow", "Green", "Blue", "Purple", "Pink"]);
  });

  it("stores keys rather than resolved copy, so a locale switch takes effect", async () => {
    await i18n.changeLanguage("pseudo");
    for (const color of TASK_COLORS) {
      const label = t(TASK_COLOR_LABEL_KEYS[color]);
      // A missing key would resolve to the key itself; frozen copy would stay
      // English. Both are ruled out by an accented, non-key label.
      expect(label).not.toBe(TASK_COLOR_LABEL_KEYS[color]);
      expect(label).toMatch(/[^\p{ASCII}]/u);
    }
  });
});

describe("backend manual task-color map parsing", () => {
  it("keeps valid colors and tombstones while dropping malformed entries", () => {
    expect(
      parseSidebarTaskColors({
        "task-red": "red",
        "task-cleared": null,
        "task-automatic-gray": "gray",
        "": "blue",
        "task-object": {},
      }),
    ).toEqual({ "task-red": "red", "task-cleared": null });
  });

  it("caps task IDs by UTF-8 byte length and entry count", () => {
    expect(parseSidebarTaskColors({ ["é".repeat(65)]: "red" })).toEqual({});
    const input = Object.fromEntries(
      Array.from({ length: 10_001 }, (_, index) => [`task-${index}`, "red"]),
    );
    expect(Object.keys(parseSidebarTaskColors(input))).toHaveLength(10_000);
  });
});
