import { describe, expect, it } from "vitest";
import { pluginDestinationId } from "./plugin-destinations";
import { resolveDestinations } from "./resolve-destinations";
import { NO_WORKSPACE_CONTEXT } from "./surface-policy";
import type { AvailabilityMap, NavContext } from "./types";
import type { PluginNavRegistration } from "@/lib/plugins/registry";

const ALL_INTEGRATIONS: AvailabilityMap = {
  "azure-devops": true,
  github: true,
  gitlab: true,
  jira: true,
  linear: true,
};

const KANBAN: NavContext = { workspaceId: "ws-1", inOffice: false };

const pluginItems: PluginNavRegistration[] = [
  { pluginId: "acme", id: "hello", label: "Hello", path: "/plugins/hello" },
  {
    pluginId: "acme",
    id: "explicit-main",
    label: "Explicit",
    path: "/plugins/explicit",
    section: "main",
  },
  {
    pluginId: "acme",
    id: "tracker",
    label: "Tracker",
    path: "/plugins/tracker",
    section: "integrations",
  },
  {
    pluginId: "acme",
    id: "prefs",
    label: "Prefs",
    path: "/settings/plugins/p",
    section: "settings",
  },
];

function ids(destinations: { id: string }[]): string[] {
  return destinations.map((destination) => destination.id);
}

function pluginsIn(surface: "sidebar" | "mobileMenu", items: PluginNavRegistration[]) {
  return resolveDestinations({
    surface,
    section: "plugins",
    ctx: NO_WORKSPACE_CONTEXT,
    pluginItems: items,
  });
}

describe("plugin destinations", () => {
  it("routes main-section items (explicit or omitted) to the plugins group", () => {
    const resolved = pluginsIn("mobileMenu", pluginItems);

    expect(ids(resolved)).toEqual(["plugin:acme:hello", "plugin:acme:explicit-main"]);
    expect(resolved.every((destination) => destination.source === "plugin")).toBe(true);
  });

  it("keeps the raw item id available for test ids", () => {
    const [hello] = pluginsIn("mobileMenu", pluginItems);

    expect(hello).toMatchObject({ id: "plugin:acme:hello", pluginItemId: "hello" });
  });

  it("routes integration-section items alongside the first-party integrations", () => {
    const resolved = resolveDestinations({
      surface: "sidebar",
      section: "integrations",
      ctx: KANBAN,
      availability: { github: true },
      pluginItems,
    });

    // First-party links keep precedence; plugin items follow.
    expect(ids(resolved)).toEqual(["github", "plugin:acme:tracker"]);
  });

  it("never renders settings-section items as destinations", () => {
    const resolved = resolveDestinations({
      surface: "mobileMenu",
      ctx: NO_WORKSPACE_CONTEXT,
      availability: ALL_INTEGRATIONS,
      pluginItems,
    });

    expect(ids(resolved)).not.toContain("plugin:acme:prefs");
  });

  it("keeps plugin items off the palette, which plugins reach via shortcuts", () => {
    const resolved = resolveDestinations({ surface: "palette", ctx: KANBAN, pluginItems });

    expect(ids(resolved)).not.toContain("plugin:acme:hello");
  });
});

/**
 * Destination ids are the React keys every nav surface renders with, and plugin
 * item ids come from third-party manifests — plugin-local, never coordinated.
 */
describe("plugin destination identity", () => {
  it("keeps a plugin from colliding with a first-party id", () => {
    const resolved = resolveDestinations({
      surface: "sidebar",
      section: "integrations",
      ctx: KANBAN,
      availability: { github: true },
      // A plugin is free to register `id: "github"` in this very section.
      pluginItems: [
        {
          pluginId: "acme",
          id: "github",
          label: "GitHub Extras",
          path: "/plugins/github-extras",
          section: "integrations",
        },
      ],
    });

    expect(ids(resolved)).toEqual(["github", "plugin:acme:github"]);
    expect(resolved[1]).toMatchObject({ pluginItemId: "github", label: "GitHub Extras" });
  });

  it("keeps two plugins that pick the same item id apart", () => {
    const resolved = pluginsIn("sidebar", [
      { pluginId: "acme", id: "dashboard", label: "Acme", path: "/plugins/acme" },
      { pluginId: "globex", id: "dashboard", label: "Globex", path: "/plugins/globex" },
    ]);

    expect(ids(resolved)).toEqual(["plugin:acme:dashboard", "plugin:globex:dashboard"]);
    expect(new Set(ids(resolved)).size).toBe(resolved.length);
    // Both keep the raw id, which is what `plugin-nav-item-<id>` test ids use.
    expect(resolved.map((destination) => destination.pluginItemId)).toEqual([
      "dashboard",
      "dashboard",
    ]);
  });

  it("encodes both segments so the separator cannot be forged", () => {
    // Without encoding, ("a:b", "c") and ("a", "b:c") would both read as
    // "plugin:a:b:c" — a collision the namespace exists to prevent.
    expect(pluginDestinationId("a:b", "c")).not.toBe(pluginDestinationId("a", "b:c"));
    expect(pluginDestinationId("a:b", "c")).toBe("plugin:a%3Ab:c");
  });
});
