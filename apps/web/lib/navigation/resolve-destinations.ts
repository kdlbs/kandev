/**
 * The one function every navigation surface calls. Filters the manifest by
 * surface, section and availability, then resolves hrefs and copy.
 *
 * Kept separate from the catalog (`core-destinations.ts`) so surfaces depend on
 * the resolution rules rather than on the data, and so the rules can be tested
 * against fixture destinations.
 */
import { APP_DESTINATIONS } from "./core-destinations";
import { pluginDestinations } from "./plugin-destinations";
import type { PluginNavRegistration } from "@/lib/plugins/registry";
import type {
  AvailabilityMap,
  Destination,
  DestinationHref,
  NavContext,
  NavSection,
  NavSurface,
  PaletteOverride,
  ResolvedDestination,
} from "./types";

export function resolveHref(href: DestinationHref, ctx: NavContext): string {
  return typeof href === "function" ? href(ctx) : href;
}

export function isAvailable(destination: Destination, availability: AvailabilityMap): boolean {
  if (!destination.requires) return true;
  return availability[destination.requires] === true;
}

export type ResolveOptions = {
  surface: NavSurface;
  /** One section, several (a surface group that renders more than one), or all. */
  section?: NavSection | NavSection[];
  ctx: NavContext;
  availability?: AvailabilityMap;
  /** i18n lookup for `labelKey` entries; identity-safe for tests. */
  translate?: (key: string) => string;
  /**
   * Plugin-registered nav items to merge in, already in registry order — as
   * `pluginRegistry.getNavRegistrations()` returns them, owner included.
   */
  pluginItems?: PluginNavRegistration[];
};

export function resolveDestinations({
  surface,
  section,
  ctx,
  availability = {},
  translate,
  pluginItems = [],
}: ResolveOptions): ResolvedDestination[] {
  const sections = section === undefined ? null : [section].flat();
  const all = [...APP_DESTINATIONS, ...pluginDestinations(pluginItems)];
  return all
    .filter((destination) => destination.surfaces.includes(surface))
    .filter((destination) => !sections || sections.includes(destination.section))
    .filter((destination) => isOfferedOnSurface(destination, surface, availability))
    .map((destination) => toResolved(destination, surface, ctx, translate));
}

function isOfferedOnSurface(
  destination: Destination,
  surface: NavSurface,
  availability: AvailabilityMap,
): boolean {
  if (surface === "palette" && destination.palette?.ignoreRequires) return true;
  return isAvailable(destination, availability);
}

function toResolved(
  destination: Destination,
  surface: NavSurface,
  ctx: NavContext,
  translate?: (key: string) => string,
): ResolvedDestination {
  const palette = surface === "palette" ? destination.palette : undefined;
  const href = resolveHref(palette?.href ?? destination.href, ctx);
  return {
    id: destination.id,
    label: destinationLabel(destination, palette, translate),
    icon: destination.icon,
    section: destination.section,
    href,
    ...(destination.source ? { source: destination.source } : {}),
    ...(destination.pluginItemId ? { pluginItemId: destination.pluginItemId } : {}),
    ...(destination.palette ? { palette: destination.palette } : {}),
  };
}

function destinationLabel(
  destination: Destination,
  palette: PaletteOverride | undefined,
  translate?: (key: string) => string,
): string {
  const key = palette?.labelKey ?? destination.labelKey;
  if (key) return translate ? translate(key) : key;
  return destination.label ?? destination.id;
}
