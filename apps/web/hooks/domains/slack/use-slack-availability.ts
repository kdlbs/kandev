"use client";

import { getSlackConfig } from "@/lib/api/domains/slack-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
  type IntegrationConfigStatus,
} from "../integrations/use-integration-availability";
import { useSlackEnabled } from "./use-slack-enabled";

// Slack stores two secrets (token + cookie) instead of one — the shared
// availability hook only checks `hasSecret`, so adapt by reporting authed
// when *both* halves are present.
async function fetchSlackStatus(): Promise<IntegrationConfigStatus | null> {
  const cfg = await getSlackConfig();
  if (!cfg) return null;
  return {
    hasSecret: cfg.hasToken && cfg.hasCookie,
    lastOk: cfg.lastOk,
  };
}

export function useSlackAuthed(): boolean {
  return useIntegrationAuthed(fetchSlackStatus);
}

export function useSlackAvailable(): boolean {
  return useIntegrationAvailable({
    useEnabled: useSlackEnabled,
    fetchConfig: fetchSlackStatus,
  });
}
