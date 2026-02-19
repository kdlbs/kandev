import { describe, expect, it, beforeEach, vi } from "vitest";
import {
  detectPlatform,
  isMac,
  formatShortcut,
  matchesShortcut,
  shortcutToCodeMirrorKeybinding,
} from "./utils";
import { KEYS, SHORTCUTS } from "./constants";
import type { KeyboardShortcut } from "./constants";

const MOZILLA_USER_AGENT = "Mozilla/5.0";

describe("detectPlatform", () => {
  beforeEach(() => {
    // Reset navigator mock before each test
    vi.unstubAllGlobals();
  });

  it("detects Mac platform from navigator.platform", () => {
    vi.stubGlobal("navigator", {
      platform: "MacIntel",
      userAgent: MOZILLA_USER_AGENT,
    });
    expect(detectPlatform()).toBe("mac");
  });

  it("detects Mac platform from userAgent", () => {
    vi.stubGlobal("navigator", {
      platform: "unknown",
      userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
    });
    expect(detectPlatform()).toBe("mac");
  });

  it("detects Windows platform", () => {
    vi.stubGlobal("navigator", {
      platform: "Win32",
      userAgent: "Mozilla/5.0 (Windows NT 10.0)",
    });
    expect(detectPlatform()).toBe("windows");
  });

  it("detects Linux platform", () => {
    vi.stubGlobal("navigator", {
      platform: "Linux x86_64",
      userAgent: "Mozilla/5.0 (X11; Linux x86_64)",
    });
    expect(detectPlatform()).toBe("linux");
  });

  it("returns unknown when navigator is undefined", () => {
    vi.stubGlobal("navigator", undefined);
    expect(detectPlatform()).toBe("unknown");
  });

  it("returns unknown for unrecognized platform", () => {
    vi.stubGlobal("navigator", {
      platform: "FreeBSD",
      userAgent: MOZILLA_USER_AGENT,
    });
    expect(detectPlatform()).toBe("unknown");
  });
});

describe("isMac", () => {
  it("returns true on Mac platform", () => {
    vi.stubGlobal("navigator", {
      platform: "MacIntel",
      userAgent: MOZILLA_USER_AGENT,
    });
    expect(isMac()).toBe(true);
  });

  it("returns false on Windows platform", () => {
    vi.stubGlobal("navigator", {
      platform: "Win32",
      userAgent: MOZILLA_USER_AGENT,
    });
    expect(isMac()).toBe(false);
  });
});

describe("formatShortcut", () => {
  it("formats Cmd+Enter on Mac", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.ENTER,
      modifiers: { ctrlOrCmd: true },
    };
    expect(formatShortcut(shortcut, "mac")).toBe("Cmd+Enter");
  });

  it("formats Ctrl+Enter on Windows", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.ENTER,
      modifiers: { ctrlOrCmd: true },
    };
    expect(formatShortcut(shortcut, "windows")).toBe("Ctrl+Enter");
  });

  it("formats Ctrl+Enter on Linux", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.ENTER,
      modifiers: { ctrlOrCmd: true },
    };
    expect(formatShortcut(shortcut, "linux")).toBe("Ctrl+Enter");
  });

  it("formats Ctrl+S", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.S,
      modifiers: { ctrl: true },
    };
    expect(formatShortcut(shortcut, "windows")).toBe("Ctrl+S");
  });

  it("formats Cmd+S on Mac", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.S,
      modifiers: { cmd: true },
    };
    expect(formatShortcut(shortcut, "mac")).toBe("Cmd+S");
  });

  it("formats Alt as Option on Mac", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.A,
      modifiers: { alt: true },
    };
    expect(formatShortcut(shortcut, "mac")).toBe("Option+A");
  });

  it("formats Alt on Windows", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.A,
      modifiers: { alt: true },
    };
    expect(formatShortcut(shortcut, "windows")).toBe("Alt+A");
  });

  it("formats Shift+Ctrl+S", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.S,
      modifiers: { ctrl: true, shift: true },
    };
    expect(formatShortcut(shortcut, "windows")).toBe("Ctrl+Shift+S");
  });

  it("formats key without modifiers", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.ESCAPE,
    };
    expect(formatShortcut(shortcut, "mac")).toBe("Esc");
  });

  it("formats special keys correctly", () => {
    expect(formatShortcut({ key: KEYS.ESCAPE }, "mac")).toBe("Esc");
    expect(formatShortcut({ key: KEYS.SPACE }, "mac")).toBe("Space");
    expect(formatShortcut({ key: KEYS.TAB }, "mac")).toBe("Tab");
    expect(formatShortcut({ key: KEYS.DELETE }, "mac")).toBe("Del");
  });

  it("formats arrow keys with symbols", () => {
    expect(formatShortcut({ key: KEYS.ARROW_UP }, "mac")).toBe("↑");
    expect(formatShortcut({ key: KEYS.ARROW_DOWN }, "mac")).toBe("↓");
    expect(formatShortcut({ key: KEYS.ARROW_LEFT }, "mac")).toBe("←");
    expect(formatShortcut({ key: KEYS.ARROW_RIGHT }, "mac")).toBe("→");
  });

  it("uppercases single letter keys", () => {
    expect(formatShortcut({ key: KEYS.A }, "mac")).toBe("A");
    expect(formatShortcut({ key: KEYS.Z }, "mac")).toBe("Z");
  });

  it("uses current platform when not specified", () => {
    vi.stubGlobal("navigator", {
      platform: "MacIntel",
      userAgent: MOZILLA_USER_AGENT,
    });
    const shortcut: KeyboardShortcut = {
      key: KEYS.ENTER,
      modifiers: { ctrlOrCmd: true },
    };
    expect(formatShortcut(shortcut)).toBe("Cmd+Enter");
  });
});

describe("matchesShortcut - matching cases", () => {
  it("matches exact key without modifiers", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.ESCAPE };
    const event = new KeyboardEvent("keydown", { key: "Escape" });
    expect(matchesShortcut(event, shortcut)).toBe(true);
  });

  it("matches ctrlOrCmd with Ctrl key", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.ENTER, modifiers: { ctrlOrCmd: true } };
    const event = new KeyboardEvent("keydown", { key: "Enter", ctrlKey: true });
    expect(matchesShortcut(event, shortcut)).toBe(true);
  });

  it("matches ctrlOrCmd with Meta key", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.ENTER, modifiers: { ctrlOrCmd: true } };
    const event = new KeyboardEvent("keydown", { key: "Enter", metaKey: true });
    expect(matchesShortcut(event, shortcut)).toBe(true);
  });

  it("matches Ctrl modifier", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.S, modifiers: { ctrl: true } };
    const event = new KeyboardEvent("keydown", { key: "s", ctrlKey: true });
    expect(matchesShortcut(event, shortcut)).toBe(true);
  });

  it("matches Cmd modifier", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.S, modifiers: { cmd: true } };
    const event = new KeyboardEvent("keydown", { key: "s", metaKey: true });
    expect(matchesShortcut(event, shortcut)).toBe(true);
  });

  it("matches Alt modifier", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.A, modifiers: { alt: true } };
    const event = new KeyboardEvent("keydown", { key: "a", altKey: true });
    expect(matchesShortcut(event, shortcut)).toBe(true);
  });

  it("matches Shift modifier", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.A, modifiers: { shift: true } };
    const event = new KeyboardEvent("keydown", { key: "a", shiftKey: true });
    expect(matchesShortcut(event, shortcut)).toBe(true);
  });

  it("matches multiple modifiers", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.S, modifiers: { ctrl: true, shift: true } };
    const event = new KeyboardEvent("keydown", { key: "s", ctrlKey: true, shiftKey: true });
    expect(matchesShortcut(event, shortcut)).toBe(true);
  });

  it("matches ctrlOrCmd with additional modifiers", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.S, modifiers: { ctrlOrCmd: true, shift: true } };
    const event = new KeyboardEvent("keydown", { key: "s", ctrlKey: true, shiftKey: true });
    expect(matchesShortcut(event, shortcut)).toBe(true);
  });

  it("works with predefined SHORTCUTS", () => {
    const event = new KeyboardEvent("keydown", { key: "Enter", metaKey: true });
    expect(matchesShortcut(event, SHORTCUTS.SUBMIT)).toBe(true);
  });
});

describe("matchesShortcut - non-matching cases", () => {
  it("does not match different key", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.ESCAPE };
    const event = new KeyboardEvent("keydown", { key: "Enter" });
    expect(matchesShortcut(event, shortcut)).toBe(false);
  });

  it("does not match ctrlOrCmd without modifier", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.ENTER, modifiers: { ctrlOrCmd: true } };
    const event = new KeyboardEvent("keydown", { key: "Enter" });
    expect(matchesShortcut(event, shortcut)).toBe(false);
  });

  it("does not match when extra modifier is pressed", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.S, modifiers: { ctrl: true } };
    const event = new KeyboardEvent("keydown", { key: "s", ctrlKey: true, shiftKey: true });
    expect(matchesShortcut(event, shortcut)).toBe(false);
  });

  it("does not match when required modifier is missing", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.S, modifiers: { ctrl: true, shift: true } };
    const event = new KeyboardEvent("keydown", { key: "s", ctrlKey: true });
    expect(matchesShortcut(event, shortcut)).toBe(false);
  });

  it("does not match when no modifiers expected but some pressed", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.ESCAPE };
    const event = new KeyboardEvent("keydown", { key: "Escape", ctrlKey: true });
    expect(matchesShortcut(event, shortcut)).toBe(false);
  });
});

describe("shortcutToCodeMirrorKeybinding", () => {
  it("formats ctrlOrCmd shortcuts with Mod", () => {
    expect(shortcutToCodeMirrorKeybinding(SHORTCUTS.SUBMIT)).toBe("Mod-Enter");
  });

  it("formats explicit modifiers", () => {
    const shortcut: KeyboardShortcut = {
      key: KEYS.S,
      modifiers: { ctrl: true, shift: true },
    };
    expect(shortcutToCodeMirrorKeybinding(shortcut)).toBe("Ctrl-Shift-s");
  });

  it("formats without modifiers", () => {
    const shortcut: KeyboardShortcut = { key: KEYS.ESCAPE };
    expect(shortcutToCodeMirrorKeybinding(shortcut)).toBe("Escape");
  });
});
