"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:github:enabled:v1";
const LEGACY_KEY_PREFIX = "kandev:github:enabled:";
const SYNC_EVENT = "kandev:github:enabled-changed";

export function useGitHubEnabled() {
  return useIntegrationEnabled(STORAGE_KEY, LEGACY_KEY_PREFIX, SYNC_EVENT);
}
