"use client";

import { fetchSentryConfig } from "@/lib/api/domains/sentry-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { qk } from "@/lib/query/keys";
import { useSentryEnabled } from "./use-sentry-enabled";

const loadSentryConfig = async () => (await fetchSentryConfig()) ?? null;

export function useSentryAuthed(): boolean {
  return useIntegrationAuthed({
    fetchConfig: loadSentryConfig,
    queryKey: qk.integrations.sentry.config(),
  });
}

export function useSentryAvailable(): boolean {
  return useIntegrationAvailable({
    useEnabled: useSentryEnabled,
    fetchConfig: loadSentryConfig,
    queryKey: qk.integrations.sentry.config(),
  });
}
