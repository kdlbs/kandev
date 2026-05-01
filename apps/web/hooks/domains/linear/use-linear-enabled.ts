"use client";

import { useIntegrationEnabled } from "../integrations/use-integration-enabled";

// Workspace-scoped: a workspace can have Linear configured but the user may
// want to silence the integration UI without removing credentials.
const storageKey = (workspaceId: string) => `kandev:linear:enabled:${workspaceId}:v1`;

const SYNC_EVENT = "kandev:linear:enabled-changed";

export function useLinearEnabled(workspaceId: string | undefined) {
  return useIntegrationEnabled(workspaceId, { storageKey, syncEvent: SYNC_EVENT });
}
