import { beforeEach, describe, expect, it } from "vitest";

import { STORAGE_KEYS } from "@/lib/settings/constants";
import {
  DEFAULT_SETTINGS_MENU_MODE,
  isTreeSettingsMenuMode,
  readSettingsMenuExpandedKeys,
  readSettingsMenuMode,
  writeSettingsMenuExpandedKeys,
  writeSettingsMenuMode,
} from "./settings-menu-mode";

const FLAT = "flat";
const AGENTS_ROW_KEY = "row:/settings/agents";
const AGENT_KEY = "agent:claude-code";

describe("settings menu mode storage", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("defaults to the flat menu when this device has never chosen", () => {
    expect(readSettingsMenuMode()).toBe(FLAT);
    expect(DEFAULT_SETTINGS_MENU_MODE).toBe(FLAT);
  });

  it("round-trips a chosen mode", () => {
    writeSettingsMenuMode("persistent");

    expect(readSettingsMenuMode()).toBe("persistent");
  });

  it("falls back rather than trusting a mode that no longer exists", () => {
    window.localStorage.setItem(STORAGE_KEYS.SETTINGS_MENU_MODE, JSON.stringify("outline"));

    expect(readSettingsMenuMode()).toBe(FLAT);
  });

  it("survives an unparseable stored value", () => {
    window.localStorage.setItem(STORAGE_KEYS.SETTINGS_MENU_MODE, "{not json");

    expect(readSettingsMenuMode()).toBe(FLAT);
  });

  it("round-trips expanded branch keys", () => {
    writeSettingsMenuExpandedKeys([AGENTS_ROW_KEY, AGENT_KEY]);

    expect(readSettingsMenuExpandedKeys()).toEqual([AGENTS_ROW_KEY, AGENT_KEY]);
  });

  it("drops non-string entries instead of feeding them to the menu", () => {
    window.localStorage.setItem(
      STORAGE_KEYS.SETTINGS_MENU_EXPANDED,
      JSON.stringify([AGENT_KEY, 7, null]),
    );

    expect(readSettingsMenuExpandedKeys()).toEqual([AGENT_KEY]);
  });

  it("treats a stored non-array as no open branches", () => {
    window.localStorage.setItem(STORAGE_KEYS.SETTINGS_MENU_EXPANDED, JSON.stringify({}));

    expect(readSettingsMenuExpandedKeys()).toEqual([]);
  });

  it("knows which modes render branches", () => {
    expect(isTreeSettingsMenuMode(FLAT)).toBe(false);
    expect(isTreeSettingsMenuMode("accordion")).toBe(true);
    expect(isTreeSettingsMenuMode("persistent")).toBe(true);
  });
});
