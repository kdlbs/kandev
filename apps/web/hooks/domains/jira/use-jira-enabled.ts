"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:jira:enabled:v1";
const LEGACY_KEY_PREFIX = "kandev:jira:enabled:";
const SYNC_EVENT = "kandev:jira:enabled-changed";

export function useJiraEnabled() {
  return useIntegrationEnabled({
    storageKey: STORAGE_KEY,
    legacyKeyPrefix: LEGACY_KEY_PREFIX,
    syncEvent: SYNC_EVENT,
  });
}
