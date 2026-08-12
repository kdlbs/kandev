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
const ACME_BOARD_ID = "plugin:acme:board";

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
  {
    pluginId: "acme",
    id: "board",
    label: "Board",
    path: "/plugins/board",
    section: "insights",
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

function insightsIn(surface: "sidebar" | "mobileMenu", items: PluginNavRegistration[]) {
  return resolveDestinations({
    surface,
    section: "insights",
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

  it("keeps plugin items off the palette", () => {
    const resolved = resolveDestinations({ surface: "palette", ctx: KANBAN, pluginItems });

    expect(ids(resolved)).not.toContain("plugin:acme:hello");
    expect(ids(resolved)).not.toContain(ACME_BOARD_ID);
  });

  it("routes insights-section items to the insights section, on both surfaces", () => {
    // The insights section also carries the first-party `stats` destination;
    // it always precedes plugin entries (see resolve-destinations.test.ts for
    // the ordering contract).
    expect(ids(insightsIn("sidebar", pluginItems))).toEqual(["stats", ACME_BOARD_ID]);
    expect(ids(insightsIn("mobileMenu", pluginItems))).toEqual(["stats", ACME_BOARD_ID]);
  });

  it("moves, does not add: an insights item is absent from the plugins group on both surfaces", () => {
    expect(ids(pluginsIn("sidebar", pluginItems))).not.toContain(ACME_BOARD_ID);
    expect(ids(pluginsIn("mobileMenu", pluginItems))).not.toContain(ACME_BOARD_ID);
  });

  it("degrades an unrecognised section string to the plugins group, never to insights", () => {
    const unknownSection = "footer" as PluginNavRegistration["section"];
    const items: PluginNavRegistration[] = [
      {
        pluginId: "acme",
        id: "mystery",
        label: "Mystery",
        path: "/plugins/mystery",
        section: unknownSection,
      },
    ];

    expect(ids(pluginsIn("sidebar", items))).toEqual(["plugin:acme:mystery"]);
    // Only the first-party `stats` destination remains — the mystery item
    // never lands in the insights section.
    expect(ids(insightsIn("sidebar", items))).toEqual(["stats"]);
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

  it("keeps two plugins' insights items apart, in registration order", () => {
    const resolved = insightsIn("sidebar", [
      {
        pluginId: "acme",
        id: "board",
        label: "Acme Board",
        path: "/plugins/acme",
        section: "insights",
      },
      {
        pluginId: "globex",
        id: "board",
        label: "Globex Board",
        path: "/plugins/globex",
        section: "insights",
      },
    ]);

    expect(ids(resolved)).toEqual(["stats", ACME_BOARD_ID, "plugin:globex:board"]);
    expect(new Set(ids(resolved)).size).toBe(resolved.length);
  });

  it("encodes both segments so the separator cannot be forged", () => {
    // Without encoding, ("a:b", "c") and ("a", "b:c") would both read as
    // "plugin:a:b:c" — a collision the namespace exists to prevent.
    expect(pluginDestinationId("a:b", "c")).not.toBe(pluginDestinationId("a", "b:c"));
    expect(pluginDestinationId("a:b", "c")).toBe("plugin:a%3Ab:c");
  });
});
