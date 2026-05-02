"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:linear:enabled:v1";
const LEGACY_KEY_PREFIX = "kandev:linear:enabled:";
const SYNC_EVENT = "kandev:linear:enabled-changed";

export function useLinearEnabled() {
  return useIntegrationEnabled(STORAGE_KEY, LEGACY_KEY_PREFIX, SYNC_EVENT);
}
