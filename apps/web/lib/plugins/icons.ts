/**
 * Curated icon-name → Tabler icon map for plugin registrations.
 *
 * The frozen plugin contract (docs/plans/plugins/PLUGIN-API.md) only carries
 * opaque string icon names (`NavItem.icon`, `PluginPageChrome.icon`) — plugins
 * never hold host component values. The host maps known names onto first-party
 * glyphs here; unknown or missing names fall back per surface (puzzle piece in
 * the sidebar, no icon in the page topbar).
 */
import {
  IconBell,
  IconBolt,
  IconBook,
  IconBug,
  IconCalendar,
  IconChartBar,
  IconChecklist,
  IconCloud,
  IconDatabase,
  IconFlask,
  IconGlobe,
  IconMessage,
  IconPuzzle,
  IconRobot,
  IconRocket,
  IconSettings,
  IconTicket,
  IconUsers,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";

/** Icon names a plugin may reference from `NavItem.icon` / `PluginPageChrome.icon`. */
export const PLUGIN_ICONS: Record<string, TablerIcon> = {
  bell: IconBell,
  bolt: IconBolt,
  book: IconBook,
  bug: IconBug,
  calendar: IconCalendar,
  chart: IconChartBar,
  checklist: IconChecklist,
  cloud: IconCloud,
  database: IconDatabase,
  flask: IconFlask,
  globe: IconGlobe,
  message: IconMessage,
  puzzle: IconPuzzle,
  robot: IconRobot,
  rocket: IconRocket,
  settings: IconSettings,
  ticket: IconTicket,
  users: IconUsers,
};

/** Strict lookup: the named icon, or undefined when the name is unknown/missing. */
export function lookupPluginIcon(name?: string): TablerIcon | undefined {
  return name ? PLUGIN_ICONS[name] : undefined;
}

/** Sidebar lookup: always renders something — unknown/missing names get the puzzle glyph. */
export function resolvePluginIcon(name?: string): TablerIcon {
  return lookupPluginIcon(name) ?? IconPuzzle;
}
