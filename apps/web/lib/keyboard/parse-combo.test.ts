import { describe, it, expect } from "vitest";
import { parseCombo } from "./parse-combo";

describe("parseCombo", () => {
  it("parses a plain single-key combo", () => {
    expect(parseCombo("k")).toEqual({ key: "k" });
  });

  it("parses mod+key as ctrlOrCmd (platform resolved by matchesShortcut)", () => {
    expect(parseCombo("mod+k")).toEqual({ key: "k", modifiers: { ctrlOrCmd: true } });
  });

  it("parses multiple modifiers plus one key", () => {
    expect(parseCombo("mod+shift+k")).toEqual({
      key: "k",
      modifiers: { ctrlOrCmd: true, shift: true },
    });
  });

  it("treats ctrl/control as an explicit ctrl modifier (not ctrlOrCmd)", () => {
    expect(parseCombo("ctrl+r")).toEqual({ key: "r", modifiers: { ctrl: true } });
    expect(parseCombo("control+r")).toEqual({ key: "r", modifiers: { ctrl: true } });
  });

  it("treats cmd/meta/super as an explicit cmd modifier", () => {
    expect(parseCombo("cmd+k")).toEqual({ key: "k", modifiers: { cmd: true } });
    expect(parseCombo("meta+k")).toEqual({ key: "k", modifiers: { cmd: true } });
    expect(parseCombo("super+k")).toEqual({ key: "k", modifiers: { cmd: true } });
  });

  it("treats alt/option as an explicit alt modifier", () => {
    expect(parseCombo("alt+k")).toEqual({ key: "k", modifiers: { alt: true } });
    expect(parseCombo("option+k")).toEqual({ key: "k", modifiers: { alt: true } });
  });

  it("resolves named key tokens to KeyboardEvent.key equivalents", () => {
    expect(parseCombo("mod+enter")).toEqual({ key: "Enter", modifiers: { ctrlOrCmd: true } });
    expect(parseCombo("shift+arrowup")).toEqual({ key: "ArrowUp", modifiers: { shift: true } });
    expect(parseCombo("shift+slash")).toEqual({ key: "/", modifiers: { shift: true } });
  });

  it("resolves function keys", () => {
    expect(parseCombo("mod+f5")).toEqual({ key: "f5", modifiers: { ctrlOrCmd: true } });
  });

  it("is case-insensitive on tokens", () => {
    expect(parseCombo("MOD+SHIFT+K")).toEqual({
      key: "k",
      modifiers: { ctrlOrCmd: true, shift: true },
    });
  });

  it("returns null for an empty combo", () => {
    expect(parseCombo("")).toBeNull();
    expect(parseCombo("   ")).toBeNull();
  });

  it("returns null for an unknown token", () => {
    expect(parseCombo("mod+banana")).toBeNull();
  });

  it("returns null when the combo has more than one non-modifier key", () => {
    expect(parseCombo("k+j")).toBeNull();
  });

  it("returns null when the combo has only modifiers and no key", () => {
    expect(parseCombo("mod+shift")).toBeNull();
  });

  it("returns null for a token with an empty segment", () => {
    expect(parseCombo("mod++k")).toBeNull();
  });
});
