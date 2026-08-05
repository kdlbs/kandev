import { describe, expect, it } from "vitest";
import { SETTINGS_MENU_SECTIONS, settingsMenuItemIsActive } from "./settings-tree";

describe("settingsMenuItemIsActive", () => {
  const agents = { href: "/settings/agents", activePrefixes: ["/settings/agents/"] };
  const executors = {
    href: "/settings/executors",
    activePrefixes: ["/settings/executors/", "/settings/executor/"],
  };
  const workspaces = {
    href: "/settings/workspaces",
    activePrefixes: ["/settings/workspaces/", "/settings/workspace/"],
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
