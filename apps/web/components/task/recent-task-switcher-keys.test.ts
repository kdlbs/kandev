import { describe, expect, it } from "vitest";
import { KEYS, type KeyboardShortcut, type Platform } from "@/lib/keyboard/constants";
import {
  getHoldModifier,
  hasHoldModifier,
  isCommitReleaseEvent,
  isCycleShortcutEvent,
} from "./recent-task-switcher-keys";

function event(overrides: Partial<KeyboardEvent>): KeyboardEvent {
  return {
    key: "",
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    shiftKey: false,
    repeat: false,
    ...overrides,
  } as KeyboardEvent;
}

const taskSwitcherShortcut: KeyboardShortcut = {
  key: KEYS.SPACE,
  modifiers: { ctrlOrCmd: true },
};

function platform(platform: Platform): Platform {
  return platform;
}

describe("recent task switcher key helpers", () => {
  it("uses Meta as the hold modifier for ctrlOrCmd on macOS", () => {
    expect(getHoldModifier(taskSwitcherShortcut, platform("mac"))).toBe("Meta");
  });

  it("uses Control as the hold modifier for ctrlOrCmd off macOS", () => {
    expect(getHoldModifier(taskSwitcherShortcut, platform("linux"))).toBe("Control");
    expect(getHoldModifier(taskSwitcherShortcut, platform("windows"))).toBe("Control");
  });

  it("detects shortcuts without a hold modifier", () => {
    expect(hasHoldModifier({ key: KEYS.SPACE })).toBe(false);
    expect(getHoldModifier({ key: KEYS.SPACE })).toBeNull();
  });

  it("matches non-repeated cycle keydown events while required modifiers are held", () => {
    expect(
      isCycleShortcutEvent(event({ key: KEYS.SPACE, ctrlKey: true }), taskSwitcherShortcut),
    ).toBe(true);
  });

  it("does not cycle on repeated keydown events", () => {
    expect(
      isCycleShortcutEvent(
        event({ key: KEYS.SPACE, ctrlKey: true, repeat: true }),
        taskSwitcherShortcut,
      ),
    ).toBe(false);
  });

  it("matches custom multi-modifier cycle shortcuts", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.Y,
      modifiers: { ctrlOrCmd: true, shift: true },
    };

    expect(isCycleShortcutEvent(event({ key: "y", ctrlKey: true, shiftKey: true }), shortcut)).toBe(
      true,
    );
    expect(isCycleShortcutEvent(event({ key: "y", ctrlKey: true }), shortcut)).toBe(false);
  });

  it("matches the hold modifier release, not secondary modifier release", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.Y,
      modifiers: { ctrlOrCmd: true, shift: true },
    };

    expect(isCommitReleaseEvent(event({ key: "Control" }), shortcut, platform("linux"))).toBe(true);
    expect(isCommitReleaseEvent(event({ key: "Shift" }), shortcut, platform("linux"))).toBe(false);
  });

  it("matches Meta release for ctrlOrCmd on macOS", () => {
    expect(
      isCommitReleaseEvent(event({ key: "Meta" }), taskSwitcherShortcut, platform("mac")),
    ).toBe(true);
    expect(
      isCommitReleaseEvent(event({ key: "Control" }), taskSwitcherShortcut, platform("mac")),
    ).toBe(false);
  });
});
