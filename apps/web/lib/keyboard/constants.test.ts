import { describe, expect, it } from "vitest";
import { MODIFIER_KEYS, KEYS, SHORTCUTS, KeyboardShortcut } from "./constants";

describe("MODIFIER_KEYS", () => {
  it("defines all modifier keys", () => {
    expect(MODIFIER_KEYS.CTRL).toBe("Control");
    expect(MODIFIER_KEYS.CMD).toBe("Meta");
    expect(MODIFIER_KEYS.ALT).toBe("Alt");
    expect(MODIFIER_KEYS.SHIFT).toBe("Shift");
  });

  it("has correct type", () => {
    // TypeScript ensures these are readonly at compile time
    const keys: typeof MODIFIER_KEYS = MODIFIER_KEYS;
    expect(keys).toBeDefined();
  });
});

describe("KEYS", () => {
  it("defines special keys", () => {
    expect(KEYS.ENTER).toBe("Enter");
    expect(KEYS.ESCAPE).toBe("Escape");
    expect(KEYS.SPACE).toBe(" ");
    expect(KEYS.TAB).toBe("Tab");
    expect(KEYS.BACKSPACE).toBe("Backspace");
    expect(KEYS.DELETE).toBe("Delete");
  });

  it("defines arrow keys", () => {
    expect(KEYS.ARROW_UP).toBe("ArrowUp");
    expect(KEYS.ARROW_DOWN).toBe("ArrowDown");
    expect(KEYS.ARROW_LEFT).toBe("ArrowLeft");
    expect(KEYS.ARROW_RIGHT).toBe("ArrowRight");
  });

  it("defines letter keys A-Z", () => {
    expect(KEYS.A).toBe("a");
    expect(KEYS.B).toBe("b");
    expect(KEYS.Z).toBe("z");
  });

  it("defines number keys 0-9", () => {
    expect(KEYS.ZERO).toBe("0");
    expect(KEYS.ONE).toBe("1");
    expect(KEYS.NINE).toBe("9");
  });

  it("has correct type", () => {
    // TypeScript ensures these are readonly at compile time
    const keys: typeof KEYS = KEYS;
    expect(keys).toBeDefined();
  });
});

describe("SHORTCUTS", () => {
  describe("SUBMIT", () => {
    it("uses Enter key with ctrlOrCmd modifier", () => {
      expect(SHORTCUTS.SUBMIT.key).toBe("Enter");
      expect(SHORTCUTS.SUBMIT.modifiers?.ctrlOrCmd).toBe(true);
    });
  });

  describe("SAVE", () => {
    it("uses S key with ctrlOrCmd modifier", () => {
      expect(SHORTCUTS.SAVE.key).toBe("s");
      expect(SHORTCUTS.SAVE.modifiers?.ctrlOrCmd).toBe(true);
    });
  });

  describe("CANCEL", () => {
    it("uses Escape key without modifiers", () => {
      const cancel = SHORTCUTS.CANCEL as KeyboardShortcut;
      expect(cancel.key).toBe("Escape");
      expect(cancel.modifiers).toBeUndefined();
    });
  });

  describe("SEARCH", () => {
    it("uses K key with ctrlOrCmd modifier", () => {
      expect(SHORTCUTS.SEARCH.key).toBe("k");
      expect(SHORTCUTS.SEARCH.modifiers?.ctrlOrCmd).toBe(true);
    });
  });

  describe("NEW_TASK", () => {
    it("uses N key with ctrlOrCmd modifier", () => {
      expect(SHORTCUTS.NEW_TASK.key).toBe("n");
      expect(SHORTCUTS.NEW_TASK.modifiers?.ctrlOrCmd).toBe(true);
    });
  });

  describe("TOGGLE_SIDEBAR", () => {
    it("uses B key with ctrlOrCmd modifier", () => {
      expect(SHORTCUTS.TOGGLE_SIDEBAR.key).toBe("b");
      expect(SHORTCUTS.TOGGLE_SIDEBAR.modifiers?.ctrlOrCmd).toBe(true);
    });
  });

  it("has correct type", () => {
    // TypeScript ensures these are readonly at compile time
    const shortcuts: typeof SHORTCUTS = SHORTCUTS;
    expect(shortcuts).toBeDefined();
  });

  it("all shortcuts have valid key property", () => {
    Object.values(SHORTCUTS).forEach((shortcut) => {
      expect(shortcut).toHaveProperty("key");
      expect(typeof shortcut.key).toBe("string");
    });
  });

  it("all shortcuts with modifiers have valid modifier properties", () => {
    Object.values(SHORTCUTS).forEach((shortcut) => {
      const typedShortcut = shortcut as KeyboardShortcut;
      if (typedShortcut.modifiers) {
        const validModifiers = ["ctrl", "cmd", "alt", "shift", "ctrlOrCmd"];
        Object.keys(typedShortcut.modifiers).forEach((modifier) => {
          expect(validModifiers).toContain(modifier);
        });
      }
    });
  });
});

describe("Type definitions", () => {
  it("KeyboardShortcut type accepts valid shortcuts", () => {
    // These should compile without errors
    const shortcut1: import("./constants").KeyboardShortcut = {
      key: "Enter",
    };
    expect(shortcut1.key).toBe("Enter");

    const shortcut2: import("./constants").KeyboardShortcut = {
      key: "s",
      modifiers: { ctrlOrCmd: true },
    };
    expect(shortcut2.modifiers?.ctrlOrCmd).toBe(true);

    const shortcut3: import("./constants").KeyboardShortcut = {
      key: "a",
      modifiers: { ctrl: true, shift: true, alt: true },
    };
    expect(shortcut3.modifiers?.ctrl).toBe(true);
  });
});
