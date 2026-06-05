"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:sentry:enabled:v1";
const LEGACY_KEY_PREFIX = "kandev:sentry:enabled:";
const SYNC_EVENT = "kandev:sentry:enabled-changed";

export function useSentryEnabled() {
  return useIntegrationEnabled(STORAGE_KEY, LEGACY_KEY_PREFIX, SYNC_EVENT);
}
