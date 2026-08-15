"use client";

import { INTEGRATION_ENABLED_KEYS } from "@/lib/integrations/integration-enabled-keys";
import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const { storageKey, legacyKeyPrefix, syncEvent } = INTEGRATION_ENABLED_KEYS["github"];

/**
 * Per-workspace enable/disable state for the GitHub integration.
 *
 * Backed by `localStorage` (key `kandev:github:enabled:v1:<workspaceId>`), synced across
 * browser tabs and across the own-settings-page and index-page sliders.
 * Defaults to `true` when no value has ever been persisted for the workspace.
 * Omitting `workspaceId` reads the pre-scoping install-wide value.
 */
export function useGitHubEnabled(workspaceId?: string | null) {
  return useIntegrationEnabled(storageKey, legacyKeyPrefix, syncEvent, workspaceId);
}
