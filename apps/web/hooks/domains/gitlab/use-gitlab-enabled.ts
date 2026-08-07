"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:gitlab:enabled:v1";
const LEGACY_KEY_PREFIX = "kandev:gitlab:enabled:";
const SYNC_EVENT = "kandev:gitlab:enabled-changed";

/**
 * Install-wide enable/disable state for the GitLab integration.
 *
 * Backed by `localStorage` (key `kandev:gitlab:enabled:v1`), synced across
 * browser tabs and across the own-settings-page and index-page sliders.
 * Defaults to `true` when no value has ever been persisted.
 */
export function useGitLabEnabled() {
  return useIntegrationEnabled(STORAGE_KEY, LEGACY_KEY_PREFIX, SYNC_EVENT);
}
