/**
 * Maps plugin-registered nav items onto destinations so plugin and first-party
 * entries render through the same components.
 *
 * Identity is the whole reason this is its own module: nav items arrive from
 * third-party manifests, and a resolved destination id is what every surface
 * uses as its React key. Two plugins may each register `id: "dashboard"`, and a
 * plugin may register an id a first-party destination already uses, so the id a
 * surface sees is built from the owning pluginId as well as the item id.
 */
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { SIDEBAR_AND_MENU } from "./surface-policy";
// Type-only: keeps this module free of the registry's React subscription.
import type { PluginNavRegistration } from "@/lib/plugins/registry";
import type { Destination } from "./types";

/**
 * Id namespace for plugin-registered entries. Matches the `plugin:<id>:…`
 * convention the app-status-bar slot ordering ids already use.
 */
const PLUGIN_ID_PREFIX = "plugin:";

/**
 * Owner-namespaced destination id for a plugin nav item.
 *
 * Both segments are percent-encoded so the separator stays unambiguous: without
 * it a plugin id of `a:b` with item `c` and a plugin id of `a` with item `b:c`
 * would produce the same id, which is exactly the collision the namespace is
 * there to prevent. The raw item id travels separately on `pluginItemId`,
 * because the `plugin-nav-item-<id>` test ids are public contract.
 */
export function pluginDestinationId(pluginId: string, itemId: string): string {
  return `${PLUGIN_ID_PREFIX}${encodeURIComponent(pluginId)}:${encodeURIComponent(itemId)}`;
}

/**
 * `section: "settings"` items are skipped — they are accepted and rendered on
 * no surface. (A plugin settings page is a separate API: `registerSettingsRoute`
 * / `registerComponent("settings-nav", …)`, not this field.)
 */
export function pluginDestinations(items: PluginNavRegistration[]): Destination[] {
  return items
    .filter((item) => (item.section ?? "main") !== "settings")
    .map((item) => ({
      id: pluginDestinationId(item.pluginId, item.id),
      pluginItemId: item.id,
      label: item.label,
      icon: resolvePluginIcon(item.icon),
      section: sectionFor(item.section),
      href: item.path,
      // No API lets a plugin declare the `palette` surface, so plugin
      // destinations never carry it — every section maps to `sidebar` +
      // `mobileMenu` only.
      surfaces: SIDEBAR_AND_MENU,
      source: "plugin" as const,
    }));
}

/**
 * `"integrations"` keeps its own manifest section; `"sidebar-footer"` (the
 * plugin-facing vocabulary) maps to the host's internal `"insights"` section.
 * Everything else — `"main"`, omitted, an unrecognised string an untyped
 * bundle might pass, or the literal `"insights"` (the host's own internal
 * name, deliberately not accepted as an alias — see spec's "insights is not
 * accepted, and not an alias") — degrades to `"plugins"`, the plugin rail /
 * Plugins group placement.
 */
function sectionFor(
  section: PluginNavRegistration["section"],
): "integrations" | "workspace-agents" | "insights" | "plugins" {
  if (section === "integrations") return "integrations";
  if (section === "after-integrations") return "workspace-agents";
  if (section === "sidebar-footer") return "insights";
  return "plugins";
}
