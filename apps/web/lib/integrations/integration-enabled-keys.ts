import { WORKSPACE_INTEGRATIONS } from "@/lib/settings-discovery/catalog/integrations";

export type IntegrationSlug = (typeof WORKSPACE_INTEGRATIONS)[number][0];

export type IntegrationEnabledKeys = {
  /**
   * Base `localStorage` key. The per-workspace value lives at
   * `<storageKey>:<workspaceId>`; the base key itself holds the pre-scoping
   * install-wide value, which now seeds a workspace that has never been
   * toggled.
   */
  storageKey: string;
  /** Pre-`v1` per-workspace keys, folded into `storageKey` on first read. */
  legacyKeyPrefix: string;
  /** Same-tab broadcast event, so both sliders re-render on a toggle. */
  syncEvent: string;
};

function keysFor(slug: string): IntegrationEnabledKeys {
  return {
    storageKey: `kandev:${slug}:enabled:v1`,
    legacyKeyPrefix: `kandev:${slug}:enabled:`,
    syncEvent: `kandev:${slug}:enabled-changed`,
  };
}

/**
 * Every integration's enable-toggle storage identity in one place.
 *
 * The per-integration `useXEnabled` hooks read their own entry, so the naming
 * scheme is declared once. Surfaces that must read *several* workspaces'
 * toggles at once (the settings menu tree) cannot call those hooks in a loop
 * and read through this catalog with `readIntegrationEnabled` instead.
 */
export const INTEGRATION_ENABLED_KEYS = Object.fromEntries(
  WORKSPACE_INTEGRATIONS.map(([slug]) => [slug, keysFor(slug)]),
) as Record<IntegrationSlug, IntegrationEnabledKeys>;

/** Every integration's same-tab toggle event, for multi-integration readers. */
export const INTEGRATION_ENABLED_SYNC_EVENTS: readonly string[] = WORKSPACE_INTEGRATIONS.map(
  ([slug]) => INTEGRATION_ENABLED_KEYS[slug].syncEvent,
);
