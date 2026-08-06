"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:azure-devops:enabled:v1";
const LEGACY_KEY_PREFIX = "kandev:azure-devops:enabled:";
const SYNC_EVENT = "kandev:azure-devops:enabled-changed";

export function useAzureDevOpsEnabled() {
  return useIntegrationEnabled(STORAGE_KEY, LEGACY_KEY_PREFIX, SYNC_EVENT);
}
