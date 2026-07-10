import { describe, it, expect } from "vitest";
import { SHORTCUTS } from "./constants";
import {
  CONFIGURABLE_SHORTCUTS,
  UNBOUND_SHORTCUT,
  getShortcut,
  isUnboundShortcut,
  resolveAllShortcuts,
  type ConfigurableShortcutId,
} from "./shortcut-overrides";

describe("CONFIGURABLE_SHORTCUTS", () => {
  it("contains all expected shortcut IDs", () => {
    const ids = Object.keys(CONFIGURABLE_SHORTCUTS);
    expect(ids).toContain("SEARCH");
    expect(ids).toContain("FILE_SEARCH");
    expect(ids).toContain("QUICK_CHAT");
    expect(ids).toContain("BOTTOM_TERMINAL");
    expect(ids).toContain("TOGGLE_SIDEBAR");
    expect(ids).toContain("COMMAND_PANEL");
    expect(ids).toContain("NEW_TASK");
    expect(ids).toContain("FOCUS_INPUT");
    expect(ids).toContain("FOCUS_PASSTHROUGH_INPUT");
    expect(ids).toContain("TOGGLE_PLAN_MODE");
    expect(ids).toContain("TASK_SWITCHER");
    expect(ids).toContain("TASK_SWITCHER_REVERSE");
    expect(ids).toContain("VOICE_INPUT_TOGGLE");
    expect(ids).toContain("REVERSE_SEARCH");
    expect(ids).toHaveLength(14);
  });

  it("each entry has a label and default matching SHORTCUTS", () => {
    expect(CONFIGURABLE_SHORTCUTS.BOTTOM_TERMINAL.label).toBe("Toggle Bottom Terminal");
    expect(CONFIGURABLE_SHORTCUTS.BOTTOM_TERMINAL.default).toBe(SHORTCUTS.BOTTOM_TERMINAL);

    expect(CONFIGURABLE_SHORTCUTS.TOGGLE_SIDEBAR.label).toBe("Toggle Sidebar");
    expect(CONFIGURABLE_SHORTCUTS.TOGGLE_SIDEBAR.default).toBe(UNBOUND_SHORTCUT);

    expect(CONFIGURABLE_SHORTCUTS.COMMAND_PANEL.label).toBe("Command Panel (Alt)");
    expect(CONFIGURABLE_SHORTCUTS.COMMAND_PANEL.default).toBe(SHORTCUTS.COMMAND_PANEL);

    expect(CONFIGURABLE_SHORTCUTS.NEW_TASK.label).toBe("New Task");
    expect(CONFIGURABLE_SHORTCUTS.NEW_TASK.default).toBe(SHORTCUTS.NEW_TASK);

    expect(CONFIGURABLE_SHORTCUTS.FOCUS_INPUT.label).toBe("Focus Chat Input");
    expect(CONFIGURABLE_SHORTCUTS.FOCUS_INPUT.default).toBe(SHORTCUTS.FOCUS_INPUT);

    expect(CONFIGURABLE_SHORTCUTS.FOCUS_PASSTHROUGH_INPUT.label).toBe("Focus CLI Chat Input");
    expect(CONFIGURABLE_SHORTCUTS.FOCUS_PASSTHROUGH_INPUT.default).toBe(
      SHORTCUTS.FOCUS_PASSTHROUGH_INPUT,
    );

    expect(CONFIGURABLE_SHORTCUTS.TOGGLE_PLAN_MODE.label).toBe("Toggle Plan Mode");
    expect(CONFIGURABLE_SHORTCUTS.TOGGLE_PLAN_MODE.default).toBe(SHORTCUTS.TOGGLE_PLAN_MODE);

    expect(CONFIGURABLE_SHORTCUTS.TASK_SWITCHER.label).toBe("Recent Task Switcher");
    expect(CONFIGURABLE_SHORTCUTS.TASK_SWITCHER.default).toBe(SHORTCUTS.TASK_SWITCHER);

    expect(CONFIGURABLE_SHORTCUTS.TASK_SWITCHER_REVERSE.label).toBe(
      "Recent Task Switcher (Backward)",
    );
    expect(CONFIGURABLE_SHORTCUTS.TASK_SWITCHER_REVERSE.default).toBe(
      SHORTCUTS.TASK_SWITCHER_REVERSE,
    );

    expect(CONFIGURABLE_SHORTCUTS.REVERSE_SEARCH.label).toBe("Reverse Chat Search");
    expect(CONFIGURABLE_SHORTCUTS.REVERSE_SEARCH.default).toBe(SHORTCUTS.REVERSE_SEARCH);
  });
});

describe("getShortcut", () => {
  it("returns default when no overrides provided", () => {
    expect(getShortcut("BOTTOM_TERMINAL")).toBe(SHORTCUTS.BOTTOM_TERMINAL);
    expect(getShortcut("TOGGLE_SIDEBAR")).toBe(UNBOUND_SHORTCUT);
  });

  it("returns default when override does not contain the ID", () => {
    expect(getShortcut("BOTTOM_TERMINAL", {})).toBe(SHORTCUTS.BOTTOM_TERMINAL);
  });

  it("returns override when present", () => {
    const override = { key: "t", modifiers: { ctrlOrCmd: true, shift: true } };
    const result = getShortcut("BOTTOM_TERMINAL", { BOTTOM_TERMINAL: override });
    expect(result).toEqual(override);
  });

  it("does not affect other shortcuts when one is overridden", () => {
    const overrides = { BOTTOM_TERMINAL: { key: "x", modifiers: { ctrlOrCmd: true } } };
    expect(getShortcut("TOGGLE_SIDEBAR", overrides)).toBe(UNBOUND_SHORTCUT);
  });
});

describe("isUnboundShortcut", () => {
  it("returns true for the sentinel and null/undefined", () => {
    expect(isUnboundShortcut(UNBOUND_SHORTCUT)).toBe(true);
    expect(isUnboundShortcut(null)).toBe(true);
    expect(isUnboundShortcut(undefined)).toBe(true);
  });

  it("returns false for a real shortcut", () => {
    expect(isUnboundShortcut(SHORTCUTS.BOTTOM_TERMINAL)).toBe(false);
  });
});

describe("resolveAllShortcuts", () => {
  it("returns all defaults when no overrides provided", () => {
    const result = resolveAllShortcuts();
    const ids = Object.keys(CONFIGURABLE_SHORTCUTS) as ConfigurableShortcutId[];
    for (const id of ids) {
      expect(result[id]).toBe(CONFIGURABLE_SHORTCUTS[id].default);
    }
  });

  it("applies overrides while keeping other defaults", () => {
    const override = { key: "m", modifiers: { alt: true } };
    const result = resolveAllShortcuts({ TOGGLE_SIDEBAR: override });
    expect(result.TOGGLE_SIDEBAR).toEqual(override);
    expect(result.BOTTOM_TERMINAL).toBe(SHORTCUTS.BOTTOM_TERMINAL);
    expect(result.SEARCH).toBe(SHORTCUTS.SEARCH);
    expect(result.TASK_SWITCHER).toBe(SHORTCUTS.TASK_SWITCHER);
  });
});
