"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

// Workspace-scoped: a workspace can have Jira configured but the user may
// want to silence the integration UI without removing credentials.
const storageKey = (workspaceId: string) => `kandev:jira:enabled:${workspaceId}:v1`;

const SYNC_EVENT = "kandev:jira:enabled-changed";

export function useJiraEnabled(workspaceId: string | undefined) {
  return useIntegrationEnabled(workspaceId, { storageKey, syncEvent: SYNC_EVENT });
}
