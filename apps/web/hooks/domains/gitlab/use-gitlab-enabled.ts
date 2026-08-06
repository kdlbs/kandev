"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:gitlab:enabled:v1";
const LEGACY_KEY_PREFIX = "kandev:gitlab:enabled:";
const SYNC_EVENT = "kandev:gitlab:enabled-changed";

export function useGitLabEnabled() {
  return useIntegrationEnabled(STORAGE_KEY, LEGACY_KEY_PREFIX, SYNC_EVENT);
}
