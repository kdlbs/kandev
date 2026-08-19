import { describe, expect, it } from "vitest";
import { pluginDestinationId } from "./plugin-destinations";
import { resolveDestinations } from "./resolve-destinations";
import { NO_WORKSPACE_CONTEXT } from "./surface-policy";
import type { AvailabilityMap, NavContext } from "./types";
import type { PluginNavRegistration } from "@/lib/plugins/registry";
import type { PluginNavSection } from "@/lib/plugins/types";
import type { NavItem } from "@/lib/plugins/types";

/**
 * Type-equality helper (standard conditional-type comparison): typechecks
 * only when A and B are the exact same type, catching a divergence in either
 * direction. See spec.md#Type-surface — the observable is equality, not
 * spelling, because TypeScript is structural and cannot distinguish a named
 * alias from an identical inline union.
 */
type Equals<A, B> =
  (<T>() => T extends A ? 1 : 2) extends <T>() => T extends B ? 1 : 2 ? true : false;

/** The host's internal `NavSection` name — never an accepted `PluginNavSection` value. */
const HOST_INTERNAL_INSIGHTS_SECTION = "insights" as const;

/** The plugin-facing `PluginNavSection` value that routes to the sidebar footer. */
const SIDEBAR_FOOTER_SECTION: PluginNavSection = "sidebar-footer";

describe("Type surface", () => {
  it("assigns all four PluginNavSection members", () => {
    const main: PluginNavSection = "main";
    const settings: PluginNavSection = "settings";
    const integrations: PluginNavSection = "integrations";
    expect([main, settings, integrations, SIDEBAR_FOOTER_SECTION]).toHaveLength(4);
  });

  it("rejects the host's internal section name via a suppressed type error", () => {
    // @ts-expect-error — the host's internal NavSection name is not an
    // accepted PluginNavSection value; if this assignment ever typechecks,
    // the unused @ts-expect-error itself becomes the error.
    const rejected: PluginNavSection = HOST_INTERNAL_INSIGHTS_SECTION;
    expect(rejected).toBe(HOST_INTERNAL_INSIGHTS_SECTION);
  });

  it("keeps NavItem['section'] and PluginNavSection in lockstep", () => {
    const equal: Equals<NavItem["section"], PluginNavSection | undefined> = true;
    expect(equal).toBe(true);
  });
});

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
    section: SIDEBAR_FOOTER_SECTION,
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
    section: HOST_INTERNAL_INSIGHTS_SECTION,
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

  it("routes sidebar-footer items to the insights section, on both surfaces", () => {
    // The insights section also carries the first-party `stats` destination;
    // it always precedes plugin entries (see resolve-destinations.test.ts for
    // the ordering contract).
    expect(ids(insightsIn("sidebar", pluginItems))).toEqual(["stats", ACME_BOARD_ID]);
    expect(ids(insightsIn("mobileMenu", pluginItems))).toEqual(["stats", ACME_BOARD_ID]);
  });

  it("moves, does not add: a sidebar-footer item is absent from the plugins group on both surfaces", () => {
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

  it("degrades the host's internal section name 'insights' to the plugins group, not an accepted alias", () => {
    // Amendment 1: the host's internal NavSection name is not a plugin-facing
    // value. It is deliberately not accepted as an alias for "sidebar-footer"
    // — see spec.md#insights-is-not-accepted-and-not-an-alias.
    const internalName = HOST_INTERNAL_INSIGHTS_SECTION as PluginNavRegistration["section"];
    const items: PluginNavRegistration[] = [
      {
        pluginId: "acme",
        id: "leaked",
        label: "Leaked",
        path: "/plugins/leaked",
        section: internalName,
      },
    ];

    expect(ids(pluginsIn("sidebar", items))).toEqual(["plugin:acme:leaked"]);
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

  it("keeps two plugins' sidebar-footer items apart, in registration order", () => {
    const resolved = insightsIn("sidebar", [
      {
        pluginId: "acme",
        id: "board",
        label: "Acme Board",
        path: "/plugins/acme",
        section: SIDEBAR_FOOTER_SECTION,
      },
      {
        pluginId: "globex",
        id: "board",
        label: "Globex Board",
        path: "/plugins/globex",
        section: SIDEBAR_FOOTER_SECTION,
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
