import { describe, expect, it } from "vitest";
import {
  SETTINGS_MENU_SECTIONS,
  settingsMenuItemIsActive,
  settingsMenuOwnerOf,
} from "./settings-tree";

const AGENTS_HREF = "/settings/agents";
const EXECUTORS_HREF = "/settings/executors";
const WORKSPACES_HREF = "/settings/workspaces";
const PLUGINS_HREF = "/settings/plugins";

describe("settingsMenuItemIsActive", () => {
  const agents = { href: AGENTS_HREF, activePrefixes: [`${AGENTS_HREF}/`] };
  const executors = {
    href: EXECUTORS_HREF,
    activePrefixes: [`${EXECUTORS_HREF}/`, "/settings/executor/"],
  };
  const workspaces = {
    href: WORKSPACES_HREF,
    activePrefixes: [`${WORKSPACES_HREF}/`, "/settings/workspace/"],
  };
  const appearance = { href: "/settings/preferences/appearance" };

  it("matches the page itself", () => {
    expect(settingsMenuItemIsActive(appearance, "/settings/preferences/appearance")).toBe(true);
    expect(settingsMenuItemIsActive(agents, "/settings/agents")).toBe(true);
  });

  it("claims detail routes for the owning page", () => {
    expect(settingsMenuItemIsActive(agents, "/settings/agents/claude/profiles/p-1")).toBe(true);
    expect(settingsMenuItemIsActive(agents, "/settings/agents/browse")).toBe(true);
    expect(settingsMenuItemIsActive(executors, "/settings/executors/profile-123")).toBe(true);
    expect(settingsMenuItemIsActive(executors, "/settings/executor/exec-1/profile/p-1")).toBe(true);
    expect(settingsMenuItemIsActive(workspaces, "/settings/workspaces/ws-1/repositories")).toBe(
      true,
    );
    expect(
      settingsMenuItemIsActive(workspaces, "/settings/workspaces/ws-1/integrations/github"),
    ).toBe(true);
  });

  it("does not match unrelated pages", () => {
    expect(settingsMenuItemIsActive(appearance, "/settings/preferences/layouts")).toBe(false);
    expect(settingsMenuItemIsActive(agents, "/settings/executors")).toBe(false);
    expect(settingsMenuItemIsActive(workspaces, "/settings/secrets")).toBe(false);
  });
});

// The settings breadcrumb names the owning row as a parent crumb, so this is
// also the answer to "which page does this route sit under".
describe("settingsMenuOwnerOf", () => {
  it("returns the row that owns a detail route", () => {
    expect(settingsMenuOwnerOf(`${WORKSPACES_HREF}/ws-1/integrations/github`)?.href).toBe(
      WORKSPACES_HREF,
    );
    expect(settingsMenuOwnerOf("/settings/executor/exec-1/profile/p-1")?.href).toBe(EXECUTORS_HREF);
    expect(settingsMenuOwnerOf(`${AGENTS_HREF}/browse`)?.href).toBe(AGENTS_HREF);
    expect(settingsMenuOwnerOf(`${PLUGINS_HREF}/kandev-plugin-e2e`)?.href).toBe(PLUGINS_HREF);
    // Legacy singular spellings redirect, and are owned by the same row.
    expect(settingsMenuOwnerOf("/settings/workspace/ws-1")?.href).toBe(WORKSPACES_HREF);
  });

  it("returns null for a page that is its own row", () => {
    // A page is not its own parent, so these get no extra crumb.
    expect(settingsMenuOwnerOf(WORKSPACES_HREF)).toBeNull();
    expect(settingsMenuOwnerOf(EXECUTORS_HREF)).toBeNull();
    expect(settingsMenuOwnerOf("/settings/preferences/appearance")).toBeNull();
    expect(settingsMenuOwnerOf("/settings")).toBeNull();
  });

  it("does not claim a workspace's secrets tab for the global Secrets page", () => {
    expect(settingsMenuOwnerOf(`${WORKSPACES_HREF}/ws-1/secrets`)?.href).toBe(WORKSPACES_HREF);
  });
});

describe("settings menu shape", () => {
  it("is exactly two levels: five static sections of plain page rows", () => {
    expect(SETTINGS_MENU_SECTIONS.map((section) => section.id)).toEqual([
      "preferences",
      "workspaces",
      "agents",
      "access",
      "system",
    ]);
    for (const section of SETTINGS_MENU_SECTIONS) {
      for (const item of section.items) {
        expect(item.href.startsWith("/settings/")).toBe(true);
      }
    }
  });

  it("holds no rows for user-created data", () => {
    const hrefs = SETTINGS_MENU_SECTIONS.flatMap((section) =>
      section.items.map((item) => item.href),
    );
    // Every row is a static path — no ids, no per-record routes.
    for (const href of hrefs) {
      expect(href).not.toMatch(/\/(ws|profile|agent)-/);
      expect(href.split("/").length).toBeLessThanOrEqual(4);
    }
    // And the row set is unique: one page, one row.
    expect(new Set(hrefs).size).toBe(hrefs.length);
  });
});
