import { beforeEach, describe, expect, it } from "vitest";
import { STORAGE_KEYS } from "./constants";
import {
  DEFAULT_SETTINGS_PATH,
  isRestorableSettingsPath,
  readLastSettingsPath,
  rememberSettingsPath,
} from "./last-settings-page";

const APPEARANCE_PATH = "/settings/general/appearance";
const PROMPTS_PATH = "/settings/prompts";
const WORKSPACE_ID = "9694f4e6-c4eb-4800-aa93-1d947907703e";
const PLUGIN_PATH = "/settings/plugins/kandev-plugin-e2e";

/** Stand-in for `SETTINGS_ROUTE_PATHS`, which the route table owns. */
const KNOWN = new Set([
  "/settings",
  APPEARANCE_PATH,
  "/settings/general/terminal",
  // A redirect stub: static, so restorable — it replaces the URL on mount and
  // the page it lands on is what gets recorded next.
  "/settings/general/shell",
  PROMPTS_PATH,
  "/settings/plugins",
  "/settings/system/status",
]);

describe("isRestorableSettingsPath", () => {
  it("accepts static settings pages", () => {
    expect(isRestorableSettingsPath(APPEARANCE_PATH, KNOWN)).toBe(true);
    expect(isRestorableSettingsPath(PROMPTS_PATH, KNOWN)).toBe(true);
    expect(isRestorableSettingsPath("/settings/system/status", KNOWN)).toBe(true);
  });

  it("accepts a redirect stub, which self-corrects on arrival", () => {
    expect(isRestorableSettingsPath("/settings/general/shell", KNOWN)).toBe(true);
  });

  it("rejects bare /settings, which is the route doing the restoring", () => {
    expect(isRestorableSettingsPath("/settings", KNOWN)).toBe(false);
    expect(isRestorableSettingsPath("/settings/", KNOWN)).toBe(false);
  });

  it("rejects a path that is not a settings route at all", () => {
    // The settings shell renders — and would therefore record — anything under
    // /settings/, including paths that fall through to the not-ported fallback.
    expect(isRestorableSettingsPath("/settings/does-not-exist", KNOWN)).toBe(false);
    expect(isRestorableSettingsPath("/stats", KNOWN)).toBe(false);
    expect(isRestorableSettingsPath("", KNOWN)).toBe(false);
  });

  it("rejects record-scoped and slug-scoped routes", () => {
    // These resolve against workspaces, agents and plugins that can be deleted.
    expect(
      isRestorableSettingsPath(`/settings/workspace/${WORKSPACE_ID}/repositories`, KNOWN),
    ).toBe(false);
    expect(isRestorableSettingsPath("/settings/agents/claude-code", KNOWN)).toBe(false);
    expect(isRestorableSettingsPath(PLUGIN_PATH, KNOWN)).toBe(false);
  });
});

describe("remember/read round trip", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("returns the default before anything is recorded", () => {
    expect(readLastSettingsPath(KNOWN)).toBe(DEFAULT_SETTINGS_PATH);
  });

  it("returns the last recorded page", () => {
    rememberSettingsPath("/settings/general/terminal", KNOWN);
    expect(readLastSettingsPath(KNOWN)).toBe("/settings/general/terminal");
  });

  it("keeps the previous page when an unrestorable one is visited", () => {
    rememberSettingsPath(PROMPTS_PATH, KNOWN);
    rememberSettingsPath(`/settings/workspace/${WORKSPACE_ID}/integrations`, KNOWN);
    rememberSettingsPath(PLUGIN_PATH, KNOWN);
    rememberSettingsPath("/settings/does-not-exist", KNOWN);

    expect(readLastSettingsPath(KNOWN)).toBe(PROMPTS_PATH);
  });

  it("normalizes a trailing slash", () => {
    rememberSettingsPath("/settings/plugins/", KNOWN);
    expect(readLastSettingsPath(KNOWN)).toBe("/settings/plugins");
  });

  it("falls back when a stored static route has since been removed", () => {
    rememberSettingsPath(PROMPTS_PATH, KNOWN);

    // The release that drops a route drops it from the table too, so the stored
    // value stops being restorable instead of resolving to the not-ported
    // fallback on every visit to /settings.
    const afterRemoval = new Set([...KNOWN].filter((path) => path !== PROMPTS_PATH));

    expect(readLastSettingsPath(afterRemoval)).toBe(DEFAULT_SETTINGS_PATH);
  });

  it("falls back when a stored plugin page's plugin is gone", () => {
    // Written by hand: `rememberSettingsPath` refuses to store this, but a value
    // left by an older build that did store slugs must not survive either.
    window.localStorage.setItem(STORAGE_KEYS.LAST_SETTINGS_PATH, JSON.stringify(PLUGIN_PATH));

    expect(readLastSettingsPath(KNOWN)).toBe(DEFAULT_SETTINGS_PATH);
  });

  it("falls back when the stored value is bare /settings", () => {
    window.localStorage.setItem(STORAGE_KEYS.LAST_SETTINGS_PATH, JSON.stringify("/settings"));
    expect(readLastSettingsPath(KNOWN)).toBe(DEFAULT_SETTINGS_PATH);
  });

  it("falls back when the stored value is not JSON", () => {
    window.localStorage.setItem(STORAGE_KEYS.LAST_SETTINGS_PATH, PROMPTS_PATH);
    expect(readLastSettingsPath(KNOWN)).toBe(DEFAULT_SETTINGS_PATH);
  });
});
