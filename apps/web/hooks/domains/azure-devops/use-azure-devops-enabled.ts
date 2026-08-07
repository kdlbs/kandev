"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:azure-devops:enabled:v1";
const LEGACY_KEY_PREFIX = "kandev:azure-devops:enabled:";
const SYNC_EVENT = "kandev:azure-devops:enabled-changed";

/**
 * Install-wide enable/disable state for the Azure DevOps integration.
 *
 * Backed by `localStorage` (key `kandev:azure-devops:enabled:v1`), synced
 * across browser tabs and across the own-settings-page and index-page
 * sliders. Defaults to `true` when no value has ever been persisted.
 */
export function useAzureDevOpsEnabled() {
  return useIntegrationEnabled(STORAGE_KEY, LEGACY_KEY_PREFIX, SYNC_EVENT);
}
