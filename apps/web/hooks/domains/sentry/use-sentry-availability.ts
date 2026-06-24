"use client";

import { listSentryInstances } from "@/lib/api/domains/sentry-api";
import {
  type IntegrationConfigStatus,
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { useSentryEnabled } from "./use-sentry-enabled";

// Sentry is "authed" when at least one configured instance has a stored token
// and a healthy last probe. Folded into the shared single-config shape so the
// integration availability hooks stay integration-agnostic.
const loadSentryConfig = async (): Promise<IntegrationConfigStatus | null> => {
  const instances = await listSentryInstances();
  const healthy = instances.some((i) => i.hasSecret && i.lastOk);
  return healthy ? { hasSecret: true, lastOk: true } : null;
};

export function useSentryAuthed(): boolean {
  return useIntegrationAuthed(loadSentryConfig);
}

export function useSentryAvailable(): boolean {
  return useIntegrationAvailable({
    useEnabled: useSentryEnabled,
    fetchConfig: loadSentryConfig,
  });
}
