/**
 * Per-surface navigation policy: which surface tuples destinations reuse, which
 * sections each surface actually draws, and the exemptions that keep the mobile
 * coverage guardrail honest. Split out from the catalog so a policy change is
 * reviewable on its own.
 *
 * Open decisions recorded here rather than encoded; see
 * `docs/decisions/2026-08-04-navigation-manifest-boundaries.md` for the durable
 * contract and tradeoffs:
 *
 * - **Per-surface ordering.** Catalog array position is currently the render
 *   order on all three surfaces at once. That is intentional while the surfaces
 *   agree; the moment one needs its own order (e.g. the palette ranking by
 *   recency, or the mobile menu promoting Settings), add a per-surface
 *   `order`/`priority` instead of reshuffling the array and silently moving rows
 *   on the other two surfaces.
 * - **Plugin path ownership.** `NavItem.path` is used verbatim as the row href,
 *   so a plugin can point a nav row at a first-party page. It cannot *serve*
 *   one: `resolvePluginRoute` in `src/spa-routes.tsx` runs after every static and
 *   nested route, so a plugin route can never shadow a first-class path. Adding
 *   a link-side ownership check needs the first-party route list as data — the
 *   same typed-route-id work the coverage guardrail wants (see
 *   `core-destinations.test.ts`) — and has to keep the legitimate case working,
 *   where a plugin links to its own `/settings/plugins/<id>` page.
 */
import type { NavContext, NavSection, NavSurface, StaticNavSection } from "./types";

// Surface tuples are shared so the same literal isn't repeated per entry (the
// duplicate-string lint counts occurrences, not intent).
export const EVERYWHERE: NavSurface[] = ["sidebar", "mobileMenu", "palette"];
export const SIDEBAR_AND_MENU: NavSurface[] = ["sidebar", "mobileMenu"];
export const MENU_AND_PALETTE: NavSurface[] = ["mobileMenu", "palette"];
export const PALETTE_ONLY: NavSurface[] = ["palette"];

/**
 * Sections the mobile menu renders, across its plugin group
 * (`MobilePluginNavSection`), its integrations group (`MobileIntegrationsSection`)
 * and its utility group (`MOBILE_MENU_UTILITY_SECTIONS`). A destination that
 * declares the `mobileMenu` surface must belong to one of these, or it would
 * claim a surface that never draws it — `core-destinations.test.ts` enforces that.
 */
export const MOBILE_MENU_SECTIONS: NavSection[] = [
  "plugins",
  "integrations",
  "insights",
  "utilities",
];

/** Sections the mobile menu's utility group renders, in order. */
export const MOBILE_MENU_UTILITY_SECTIONS: StaticNavSection[] = ["insights", "utilities"];

/**
 * Destinations deliberately not offered in the mobile menu, each with the mobile
 * affordance that owns it instead. Everything else must be offered there: the
 * sidebar is hidden below `md`, so an omission means unreachable on a phone —
 * which is exactly how `/stats` ended up with no phone entry point.
 * `core-destinations.test.ts` fails on any omission that is not listed here.
 */
export const MOBILE_MENU_EXEMPTIONS: Record<string, string> = {
  home: "the mobile header's brand link is the phone's home affordance",
  tasks: "the mobile menu's View toggle switches between Kanban and List",
};

/** Catalog key the palette groups navigation commands under. */
export const PALETTE_NAVIGATION_GROUP_KEY = "common:commandGroupNavigation";

/**
 * Context for surfaces whose destinations carry static hrefs (the plugins group)
 * — lets them resolve without pulling workspace/mode hooks they don't need.
 */
export const NO_WORKSPACE_CONTEXT: NavContext = { workspaceId: null, inOffice: false };
