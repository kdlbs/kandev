import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { APP_DESTINATIONS } from "./core-destinations";
import { resolveDestinations } from "./resolve-destinations";
import {
  MOBILE_MENU_EXEMPTIONS,
  MOBILE_MENU_SECTIONS,
  NO_WORKSPACE_CONTEXT,
} from "./surface-policy";
import { GATED_SECTIONS, type AvailabilityMap } from "./types";

const ALL_INTEGRATIONS: AvailabilityMap = {
  "azure-devops": true,
  github: true,
  gitlab: true,
  jira: true,
  linear: true,
};

function ids(destinations: { id: string }[]): string[] {
  return destinations.map((destination) => destination.id);
}

describe("manifest invariants", () => {
  it("gives every destination a unique id", () => {
    expect(new Set(ids(APP_DESTINATIONS)).size).toBe(APP_DESTINATIONS.length);
  });

  it("gives every destination exactly one of label or labelKey", () => {
    // Also a compile error (`DestinationCopy`); kept as a runtime check because
    // an entry can still reach this array through a cast.
    for (const destination of APP_DESTINATIONS) {
      expect(
        Boolean(destination.label) !== Boolean(destination.labelKey),
        `${destination.id} must set either label (brand name) or labelKey (translated copy)`,
      ).toBe(true);
    }
  });

  it("declares at least one surface per destination", () => {
    for (const destination of APP_DESTINATIONS) {
      expect(destination.surfaces.length, `${destination.id} is offered nowhere`).toBeGreaterThan(
        0,
      );
    }
  });

  it("gives every palette destination a command id and copy", () => {
    const paletteDestinations = APP_DESTINATIONS.filter((destination) =>
      destination.surfaces.includes("palette"),
    );

    expect(paletteDestinations.length).toBeGreaterThan(0);
    for (const destination of paletteDestinations) {
      expect(destination.palette?.id, `${destination.id} needs a stable command id`).toBeTruthy();
      expect(destination.palette?.labelKey, `${destination.id} needs command copy`).toBeTruthy();
    }
  });
});

/**
 * The guardrails that make the manifest worth having. Both encode bugs the app
 * actually had: `/stats` reachable only from the desktop sidebar, and new routes
 * shipping with no navigation entry at all.
 */
describe("navigation coverage guardrails", () => {
  it("offers every destination in the mobile menu unless it is explicitly exempt", () => {
    // Deliberately stricter than "sidebar destinations must also be mobile":
    // a palette-only destination is just as unreachable on a phone, so every
    // omission has to be justified in MOBILE_MENU_EXEMPTIONS.
    const missing = APP_DESTINATIONS.filter(
      (destination) =>
        !destination.surfaces.includes("mobileMenu") && !(destination.id in MOBILE_MENU_EXEMPTIONS),
    );

    expect(
      ids(missing),
      "the sidebar is hidden below md, so a destination that is not in the mobile menu is unreachable on a phone — add the mobileMenu surface, or record the mobile affordance that owns it in MOBILE_MENU_EXEMPTIONS",
    ).toEqual([]);
  });

  it("keeps the exemption list honest", () => {
    for (const [id, reason] of Object.entries(MOBILE_MENU_EXEMPTIONS)) {
      const destination = APP_DESTINATIONS.find((entry) => entry.id === id);
      expect(destination, `${id} is exempt but is not a destination`).toBeTruthy();
      expect(
        destination?.surfaces.includes("mobileMenu"),
        `${id} is offered in the mobile menu, so it should not be exempt`,
      ).toBe(false);
      expect(reason.length, `${id} needs a reason naming the surface that owns it`).toBeGreaterThan(
        10,
      );
    }
  });

  it("confines availability gates to the sections that resolve availability", () => {
    // `useStaticDestinations` skips the availability subscription (which polls
    // per consumer). That is only safe while gates stay inside GATED_SECTIONS.
    const strays = APP_DESTINATIONS.filter(
      (destination) =>
        destination.requires && !GATED_SECTIONS.some((section) => section === destination.section),
    );

    expect(
      ids(strays),
      `a gated destination outside ${GATED_SECTIONS.join(", ")} would silently disappear on surfaces that use useStaticDestinations`,
    ).toEqual([]);
  });

  it("keeps every palette destination resolvable without availability", () => {
    // The palette resolves statically, so a gated entry must opt out explicitly.
    const unresolvable = APP_DESTINATIONS.filter(
      (destination) =>
        destination.surfaces.includes("palette") &&
        destination.requires &&
        !destination.palette?.ignoreRequires,
    );

    expect(
      ids(unresolvable),
      "a gated palette entry needs palette.ignoreRequires, or the palette must switch to useAppDestinations",
    ).toEqual([]);
  });

  it("puts every mobile-menu destination in a section the mobile menu renders", () => {
    const unrendered = APP_DESTINATIONS.filter(
      (destination) =>
        destination.surfaces.includes("mobileMenu") &&
        !MOBILE_MENU_SECTIONS.includes(destination.section),
    );

    expect(
      ids(unrendered),
      `mobileMenu destinations must live in one of: ${MOBILE_MENU_SECTIONS.join(", ")}`,
    ).toEqual([]);
  });

  it("covers every first-class top-level route with a destination", () => {
    // Pre-auth routes: the SPA shell bounces them, so they are never navigation
    // targets. `/` (the kanban catch-all) and the nested `/settings` and
    // `/office` trees are not part of this switch; Office is still owned by the
    // sidebar footer's mode toggle rather than a manifest destination.
    const NOT_NAVIGABLE = ["/login", "/setup", "/invite"];

    // Source-scanned rather than imported: the route table is a `switch` over
    // string literals, so there is no route list to read at runtime. Typed route
    // ids shared by routing and navigation would replace this — see the
    // open-decisions note in `surface-policy.ts`. Until then the length floor
    // below turns a restructured switch into a failure rather than a silent pass.
    const here = path.dirname(fileURLToPath(import.meta.url));
    const source = readFileSync(path.join(here, "../../src/spa-routes.tsx"), "utf8");
    const topLevelRoutes = [...source.matchAll(/case "(\/[a-z0-9-]*)":/g)].map((match) => match[1]);

    expect(
      topLevelRoutes.length,
      "no top-level routes were found in src/spa-routes.tsx — the switch was restructured, so this guardrail needs updating rather than passing vacuously",
    ).toBeGreaterThan(5);

    const covered = new Set(
      resolveDestinations({
        surface: "sidebar",
        ctx: NO_WORKSPACE_CONTEXT,
        availability: ALL_INTEGRATIONS,
      })
        .concat(
          resolveDestinations({
            surface: "palette",
            ctx: NO_WORKSPACE_CONTEXT,
            availability: ALL_INTEGRATIONS,
          }),
        )
        .map((destination) => destination.href.split("?")[0]),
    );

    const uncovered = topLevelRoutes.filter(
      (route) => !NOT_NAVIGABLE.includes(route) && !covered.has(route),
    );

    expect(
      uncovered,
      "every first-class route needs a navigation manifest entry (or an explicit NOT_NAVIGABLE reason)",
    ).toEqual([]);
  });
});
