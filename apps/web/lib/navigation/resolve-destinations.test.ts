import { describe, expect, it } from "vitest";
import { APP_DESTINATIONS } from "./core-destinations";
import { resolveDestinations } from "./resolve-destinations";
import { MOBILE_MENU_UTILITY_SECTIONS } from "./surface-policy";
import type { AvailabilityMap, NavContext } from "./types";

const ALL_INTEGRATIONS: AvailabilityMap = {
  "azure-devops": true,
  github: true,
  gitlab: true,
  jira: true,
  linear: true,
};

const KANBAN: NavContext = { workspaceId: "ws-1", inOffice: false };
const OFFICE: NavContext = { workspaceId: "ws-office", inOffice: true };

function ids(destinations: { id: string }[]): string[] {
  return destinations.map((destination) => destination.id);
}

describe("resolveDestinations", () => {
  it("offers the sidebar's insight and integration destinations, in manifest order", () => {
    const resolved = resolveDestinations({
      surface: "sidebar",
      ctx: KANBAN,
      availability: ALL_INTEGRATIONS,
    });

    expect(ids(resolved)).toEqual(["stats", "azure-devops", "github", "gitlab", "jira", "linear"]);
  });

  it("hides integrations that are not configured", () => {
    const resolved = resolveDestinations({
      surface: "sidebar",
      section: "integrations",
      ctx: KANBAN,
      availability: { github: true, linear: true },
    });

    expect(ids(resolved)).toEqual(["github", "linear"]);
    expect(resolved.map((destination) => destination.href)).toEqual(["/github", "/linear"]);
  });

  it("offers no integrations when none are configured", () => {
    const resolved = resolveDestinations({
      surface: "sidebar",
      section: "integrations",
      ctx: KANBAN,
      availability: {},
    });

    expect(resolved).toEqual([]);
  });

  it("accepts several sections, which is how the mobile menu's utility group resolves", () => {
    const resolved = resolveDestinations({
      surface: "mobileMenu",
      section: MOBILE_MENU_UTILITY_SECTIONS,
      ctx: KANBAN,
    });

    expect(ids(resolved)).toEqual(["stats", "settings"]);
  });

  it("resolves workspace-dependent hrefs from the nav context", () => {
    const [home] = resolveDestinations({ surface: "palette", section: "primary", ctx: KANBAN });

    expect(home?.href).toBe("/?home=overview");
  });

  it("follows the active mode for destinations whose href depends on it", () => {
    // Home is palette-only, so assert the context wiring through the manifest
    // entry itself rather than a surface that does not offer it.
    const home = APP_DESTINATIONS.find((destination) => destination.id === "home");
    const href = home?.href;

    expect(typeof href).toBe("function");
    expect(typeof href === "function" ? href(OFFICE) : null).toBe("/office");
    expect(typeof href === "function" ? href(KANBAN) : null).toBe(
      "/?home=overview&workspaceId=ws-1",
    );
  });

  it("resolves labels through the injected translator, leaving brand names alone", () => {
    const resolved = resolveDestinations({
      surface: "sidebar",
      ctx: KANBAN,
      availability: ALL_INTEGRATIONS,
      translate: (key) => `t(${key})`,
    });

    expect(resolved[0]).toMatchObject({ id: "stats", label: "t(sidebar:stats)" });
    expect(resolved.find((destination) => destination.id === "github")?.label).toBe("GitHub");
  });

  it("applies palette overrides for id, copy and href", () => {
    const resolved = resolveDestinations({ surface: "palette", ctx: KANBAN });
    const settings = resolved.find((destination) => destination.id === "settings");

    expect(settings?.href).toBe("/settings/general");
    expect(settings?.label).toBe("common:commandGoToSettings");
    expect(settings?.palette?.id).toBe("nav-settings");
  });

  it("keeps the surface-agnostic href outside the palette", () => {
    const [settings] = resolveDestinations({
      surface: "mobileMenu",
      section: "utilities",
      ctx: KANBAN,
    });

    expect(settings).toMatchObject({ id: "settings", href: "/settings" });
  });

  it("keeps the GitHub command listed when GitHub is unconfigured, unlike the sidebar link", () => {
    const palette = resolveDestinations({ surface: "palette", ctx: KANBAN, availability: {} });
    const sidebar = resolveDestinations({ surface: "sidebar", ctx: KANBAN, availability: {} });

    expect(ids(palette)).toContain("github");
    expect(ids(sidebar)).not.toContain("github");
  });
});
