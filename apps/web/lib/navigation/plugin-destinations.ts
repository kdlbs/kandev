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
 * `section: "settings"` items are skipped — those render in the settings tree's
 * `PluginSlot`, not as destinations.
 */
export function pluginDestinations(items: PluginNavRegistration[]): Destination[] {
  return items
    .filter((item) => (item.section ?? "main") !== "settings")
    .map((item) => ({
      id: pluginDestinationId(item.pluginId, item.id),
      pluginItemId: item.id,
      label: item.label,
      icon: resolvePluginIcon(item.icon),
      section: item.section === "integrations" ? ("integrations" as const) : ("plugins" as const),
      href: item.path,
      // Plugins reach the palette through `registerShortcut`, not as nav
      // commands, so plugin destinations stay off that surface.
      surfaces: SIDEBAR_AND_MENU,
      source: "plugin" as const,
    }));
}
