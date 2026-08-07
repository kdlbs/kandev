/**
 * Shape of the navigation manifest — the vocabulary every other module in
 * `lib/navigation/` and every nav surface speaks.
 *
 * The catalog itself lives in `core-destinations.ts`, the plugin mapping in
 * `plugin-destinations.ts`, the per-surface constants in `surface-policy.ts`, and
 * the one filter/resolve entry point in `resolve-destinations.ts`. All of it is
 * deliberately pure — no React, no hooks, no `t()`. Availability (integration
 * configured?) and copy resolution are injected by
 * `hooks/use-app-destinations.ts`, which keeps the manifest unit-testable.
 */
import type { ComponentType } from "react";

/**
 * Where a destination may be offered; every destination lists its surfaces
 * explicitly rather than defaulting, so an omission is visible in review.
 *
 * - `sidebar` — desktop navigation chrome: the `AppSidebar` sections and footer,
 *   plus the integration dropdown/topbar shortcuts that mirror them. All of it is
 *   hidden below the `md` breakpoint.
 * - `mobileMenu` — the phone/tablet hamburger sheet, which is the only navigation
 *   surface below `md`.
 * - `palette` — the command panel's Navigation group (keyboard-only today).
 */
export type NavSurface = "sidebar" | "mobileMenu" | "palette";

/**
 * Grouping within a surface. `plugins` and `integrations` keep their own group
 * headings in the mobile menu and their own sidebar sections; `insights` and
 * `utilities` render together in the mobile menu's utility block.
 */
export type NavSection = "primary" | "plugins" | "integrations" | "insights" | "utilities";

/**
 * Sections that may contain availability-gated destinations. Only surfaces
 * rendering these sections need `useAppDestinations`; everything else can use
 * `useStaticDestinations` and skip the integration availability subscription,
 * which polls per consumer. `core-destinations.test.ts` enforces that no
 * destination outside these sections declares `requires`.
 *
 * A value rather than a plain union because both the type and the runtime
 * guardrail derive from it.
 */
export const GATED_SECTIONS = ["integrations"] as const satisfies readonly NavSection[];

export type GatedNavSection = (typeof GATED_SECTIONS)[number];
export type StaticNavSection = Exclude<NavSection, GatedNavSection>;

/** Keys the availability map may carry. Extend when a gated destination is added. */
export type AvailabilityKey = "azure-devops" | "github" | "gitlab" | "jira" | "linear";

export type DestinationIcon = ComponentType<{ className?: string }>;

/** Resolved at render time so hrefs can follow the active workspace / mode. */
export type NavContext = {
  workspaceId: string | null;
  inOffice: boolean;
};

export type DestinationHref = string | ((ctx: NavContext) => string);

/** Palette-specific overrides. Command ids are stable API — tests select by them. */
export type PaletteOverride = {
  id: string;
  labelKey: string;
  keywordsKey?: string;
  /** Palette href when it differs from the surface-agnostic one. */
  href?: DestinationHref;
  /** Offered even when `requires` is unmet (preserves pre-manifest behavior). */
  ignoreRequires?: boolean;
};

/**
 * Brand and product names are never translated (see apps/web/AGENTS.md), so
 * integrations and plugin entries carry a literal `label` while first-party copy
 * carries a catalog `labelKey`. Encoded as a union so "exactly one of the two"
 * is a compile error rather than only a test failure.
 */
export type DestinationCopy =
  | { label: string; labelKey?: never }
  | { label?: never; labelKey: string };

type DestinationBase = {
  /**
   * Stable identity, unique across the manifest and the merged plugin entries.
   * Plugin ids are owner-namespaced (`plugin:<pluginId>:<NavItem.id>`, see
   * `pluginDestinationId`) so neither a first-party id nor another plugin's item
   * id can collide with them. Consumers use this as the React key.
   */
  id: string;
  /** Raw `NavItem.id` for plugin entries; never set on first-party entries. */
  pluginItemId?: string;
  icon: DestinationIcon;
  section: NavSection;
  href: DestinationHref;
  /**
   * Surfaces that may offer this destination. Array position in the catalog is
   * the render order on every surface today; see the open-decisions note in
   * `surface-policy.ts` if a surface ever needs its own order.
   */
  surfaces: NavSurface[];
  /** Availability gate; omitted means always offered. */
  requires?: AvailabilityKey;
  palette?: PaletteOverride;
  /** "plugin" entries get surface-specific test ids; see `pluginDestinations`. */
  source?: "plugin";
};

export type Destination = DestinationBase & DestinationCopy;

export type ResolvedDestination = {
  id: string;
  label: string;
  icon: DestinationIcon;
  section: NavSection;
  href: string;
  source?: "plugin";
  /** Raw `NavItem.id` for plugin entries — the id e2e test ids are built from. */
  pluginItemId?: string;
  palette?: PaletteOverride;
};

export type AvailabilityMap = Partial<Record<AvailabilityKey, boolean>>;
