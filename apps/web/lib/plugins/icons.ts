/**
 * Curated icon-name → Tabler icon map for plugin registrations.
 *
 * Registrations may use a curated opaque name or a plugin-owned component.
 * The host maps known names onto first-party glyphs and passes component icons
 * through; unknown or missing names retain each surface's existing fallback
 * (puzzle piece in the sidebar, no icon in the page topbar).
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
import type { PluginIcon } from "./types";

type PluginIconComponent = Exclude<PluginIcon, string>;
type ResolvedPluginIcon = TablerIcon | PluginIconComponent;

/** Curated icon names a plugin may reference instead of supplying its own component. */
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
export function lookupPluginIcon(icon?: PluginIcon): ResolvedPluginIcon | undefined {
  if (typeof icon === "function") return icon;
  return icon ? PLUGIN_ICONS[icon] : undefined;
}

/** Sidebar lookup: always renders something — unknown/missing names get the puzzle glyph. */
export function resolvePluginIcon(icon?: PluginIcon): ResolvedPluginIcon {
  return lookupPluginIcon(icon) ?? IconPuzzle;
}
