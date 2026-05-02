"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:jira:enabled:v1";
const LEGACY_KEY_PREFIX = "kandev:jira:enabled:";
const SYNC_EVENT = "kandev:jira:enabled-changed";

export function useJiraEnabled() {
  return useIntegrationEnabled(STORAGE_KEY, LEGACY_KEY_PREFIX, SYNC_EVENT);
}
