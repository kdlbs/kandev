import { beforeEach, describe, expect, it } from "vitest";
import { STORAGE_KEYS } from "./constants";
import {
  DEFAULT_SETTINGS_PATH,
  isRememberableSettingsPath,
  readLastSettingsPath,
  rememberSettingsPath,
} from "./last-settings-page";

const WORKSPACE_ID = "9694f4e6-c4eb-4800-aa93-1d947907703e";

describe("isRememberableSettingsPath", () => {
  it("accepts static settings pages", () => {
    expect(isRememberableSettingsPath("/settings/general/appearance")).toBe(true);
    expect(isRememberableSettingsPath("/settings/prompts")).toBe(true);
    expect(isRememberableSettingsPath("/settings/system/status")).toBe(true);
  });

  it("rejects bare /settings, which is the route doing the restoring", () => {
    expect(isRememberableSettingsPath("/settings")).toBe(false);
    expect(isRememberableSettingsPath("/settings/")).toBe(false);
  });

  it("rejects paths carrying a record id", () => {
    // Restoring one of these after the workspace or agent is gone lands the
    // user on a broken page; the default is a better answer.
    expect(isRememberableSettingsPath(`/settings/workspace/${WORKSPACE_ID}/repositories`)).toBe(
      false,
    );
    expect(isRememberableSettingsPath("/settings/agents/1a2b3c4d5e")).toBe(false);
  });

  it("rejects paths outside settings", () => {
    expect(isRememberableSettingsPath("/stats")).toBe(false);
    expect(isRememberableSettingsPath("/settingsx/foo")).toBe(false);
    expect(isRememberableSettingsPath("")).toBe(false);
  });

  it("accepts a redirect stub, which self-corrects on arrival", () => {
    // `/settings/general/shell` replaces the URL with /settings/general/terminal
    // on mount, so the next thing recorded is the real page.
    expect(isRememberableSettingsPath("/settings/general/shell")).toBe(true);
  });
});

describe("remember/read round trip", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("returns the default before anything is recorded", () => {
    expect(readLastSettingsPath()).toBe(DEFAULT_SETTINGS_PATH);
  });

  it("returns the last recorded page", () => {
    rememberSettingsPath("/settings/general/terminal");
    expect(readLastSettingsPath()).toBe("/settings/general/terminal");
  });

  it("keeps the previous page when an unrestorable one is visited", () => {
    rememberSettingsPath("/settings/prompts");
    rememberSettingsPath(`/settings/workspace/${WORKSPACE_ID}/integrations`);
    expect(readLastSettingsPath()).toBe("/settings/prompts");
  });

  it("normalizes a trailing slash", () => {
    rememberSettingsPath("/settings/plugins/");
    expect(readLastSettingsPath()).toBe("/settings/plugins");
  });

  it("falls back when the stored value is no longer restorable", () => {
    // A value written by an older release, or by hand.
    window.localStorage.setItem(STORAGE_KEYS.LAST_SETTINGS_PATH, JSON.stringify("/settings"));
    expect(readLastSettingsPath()).toBe(DEFAULT_SETTINGS_PATH);
  });

  it("falls back when the stored value is not JSON", () => {
    window.localStorage.setItem(STORAGE_KEYS.LAST_SETTINGS_PATH, "/settings/prompts");
    expect(readLastSettingsPath()).toBe(DEFAULT_SETTINGS_PATH);
  });
});
