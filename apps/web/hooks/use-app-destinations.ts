"use client";

import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useInOffice } from "@/hooks/use-in-office";
import { useNavAvailability } from "@/hooks/use-nav-availability";
import { resolveDestinations } from "@/lib/navigation/resolve-destinations";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type {
  GatedNavSection,
  NavContext,
  NavSurface,
  ResolvedDestination,
  StaticNavSection,
} from "@/lib/navigation/types";

/** Active workspace and mode, the inputs a destination href may depend on. */
export function useNavContext(): NavContext {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const inOffice = useInOffice();
  return { workspaceId, inOffice };
}

/**
 * Destinations for a surface that renders availability-gated sections — today
 * that means the integrations groups (see `GATED_SECTIONS`).
 *
 * Subscribes to `useNavAvailability`, and therefore to the per-integration auth
 * probes: Jira, Linear and Azure DevOps each run a fetch plus a 90s
 * `setInterval` per mounted consumer
 * (`hooks/domains/integrations/use-integration-availability.ts`). Use
 * `useStaticDestinations` for ungated sections so a nav surface doesn't add
 * background polling just to render a static link.
 *
 * Not memoized: the availability map is rebuilt each render, so a `useMemo` here
 * would never hit. Consumers map the result straight into JSX.
 */
export function useAppDestinations(
  surface: NavSurface,
  section: GatedNavSection | GatedNavSection[],
): ResolvedDestination[] {
  const { t } = useTranslation();
  const ctx = useNavContext();
  const availability = useNavAvailability();
  const registry = usePluginRegistry();

  return resolveDestinations({
    surface,
    ...(section ? { section } : {}),
    ctx,
    availability,
    translate: t,
    pluginItems: registry.getNavRegistrations(),
  });
}

/**
 * Destinations for a surface whose sections carry no availability gate — the
 * sidebar footer, the mobile menu's utility group, and the command palette
 * (whose one gated entry opts out via `palette.ignoreRequires`).
 *
 * Identical to `useAppDestinations` minus the availability subscription, so
 * these surfaces cost no background requests. `core-destinations.test.ts` enforces
 * the invariant that makes this safe: nothing outside `GATED_SECTIONS` declares
 * `requires`, and every palette entry is either ungated or opted out.
 *
 * Only the palette may omit `section` (first overload). Omitting it resolves
 * every section, gated ones included, against an empty availability map — which
 * is correct only because the palette's single gated entry opts out via
 * `palette.ignoreRequires`. On any other surface that would silently drop every
 * unconfigured integration, so the section is required there.
 */
export function useStaticDestinations(surface: "palette"): ResolvedDestination[];
export function useStaticDestinations(
  surface: NavSurface,
  section: StaticNavSection | StaticNavSection[],
): ResolvedDestination[];
export function useStaticDestinations(
  surface: NavSurface,
  section?: StaticNavSection | StaticNavSection[],
): ResolvedDestination[] {
  const { t } = useTranslation();
  const ctx = useNavContext();
  const registry = usePluginRegistry();

  return resolveDestinations({
    surface,
    ...(section ? { section } : {}),
    ctx,
    translate: t,
    pluginItems: registry.getNavRegistrations(),
  });
}
