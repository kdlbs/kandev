import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  getTaskColor,
  setTaskColor,
  TASK_COLORS,
  TASK_COLOR_BAR_CLASS,
  TASK_COLOR_LABEL_KEYS,
  TASK_COLORS_CHANGED_EVENT,
  TASK_COLORS_STORAGE_KEY,
} from "./task-colors";
import { i18n, t } from "@/lib/i18n";

describe("task colors storage", () => {
  beforeEach(() => {
    window.localStorage.clear();
    // Synthesize a cross-tab storage event so the module-level cache invalidates,
    // matching how a real browser would notify other tabs after localStorage.clear().
    window.dispatchEvent(new StorageEvent("storage", { key: null }));
  });

  it("returns null when no color is stored", () => {
    expect(getTaskColor("task-1")).toBeNull();
  });

  it("stores and reads a color", () => {
    setTaskColor("task-1", "blue");
    expect(getTaskColor("task-1")).toBe("blue");
  });

  it("removes a color when set to null", () => {
    setTaskColor("task-1", "blue");
    setTaskColor("task-1", null);
    expect(getTaskColor("task-1")).toBeNull();
  });

  it("ignores invalid colors loaded from storage", () => {
    window.localStorage.setItem(
      TASK_COLORS_STORAGE_KEY,
      JSON.stringify({ "task-1": "fuchsia", "task-2": "red" }),
    );
    expect(getTaskColor("task-1")).toBeNull();
    expect(getTaskColor("task-2")).toBe("red");
  });

  it("returns null on malformed storage", () => {
    window.localStorage.setItem(TASK_COLORS_STORAGE_KEY, "{not json");
    expect(getTaskColor("task-1")).toBeNull();
  });

  it("dispatches a change event when a color is set", () => {
    const listener = vi.fn();
    window.addEventListener(TASK_COLORS_CHANGED_EVENT, listener);
    setTaskColor("task-1", "green");
    expect(listener).toHaveBeenCalledTimes(1);
    window.removeEventListener(TASK_COLORS_CHANGED_EVENT, listener);
  });

  it("does not dispatch when setting the same color twice", () => {
    setTaskColor("task-1", "green");
    const listener = vi.fn();
    window.addEventListener(TASK_COLORS_CHANGED_EVENT, listener);
    setTaskColor("task-1", "green");
    expect(listener).not.toHaveBeenCalled();
    window.removeEventListener(TASK_COLORS_CHANGED_EVENT, listener);
  });

  it("does not dispatch when clearing a non-existent color", () => {
    const listener = vi.fn();
    window.addEventListener(TASK_COLORS_CHANGED_EVENT, listener);
    setTaskColor("task-1", null);
    expect(listener).not.toHaveBeenCalled();
    window.removeEventListener(TASK_COLORS_CHANGED_EVENT, listener);
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
