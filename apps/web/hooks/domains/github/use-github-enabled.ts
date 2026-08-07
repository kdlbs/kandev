"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:github:enabled:v1";
const LEGACY_KEY_PREFIX = "kandev:github:enabled:";
const SYNC_EVENT = "kandev:github:enabled-changed";

/**
 * Install-wide enable/disable state for the GitHub integration.
 *
 * Backed by `localStorage` (key `kandev:github:enabled:v1`), synced across
 * browser tabs and across the own-settings-page and index-page sliders.
 * Defaults to `true` when no value has ever been persisted.
 */
export function useGitHubEnabled() {
  return useIntegrationEnabled(STORAGE_KEY, LEGACY_KEY_PREFIX, SYNC_EVENT);
}
