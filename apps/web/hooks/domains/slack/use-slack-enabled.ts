"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:slack:enabled:v1";
const LEGACY_KEY_PREFIX = "kandev:slack:enabled:";
const SYNC_EVENT = "kandev:slack:enabled-changed";

export function useSlackEnabled() {
  return useIntegrationEnabled(STORAGE_KEY, LEGACY_KEY_PREFIX, SYNC_EVENT);
}
