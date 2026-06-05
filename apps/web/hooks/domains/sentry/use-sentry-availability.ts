"use client";

import { fetchSentryConfig } from "@/lib/api/domains/sentry-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { useSentryEnabled } from "./use-sentry-enabled";

const loadSentryConfig = async () => (await fetchSentryConfig()) ?? null;

export function useSentryAuthed(): boolean {
  return useIntegrationAuthed(loadSentryConfig);
}

export function useSentryAvailable(): boolean {
  return useIntegrationAvailable({
    useEnabled: useSentryEnabled,
    fetchConfig: loadSentryConfig,
  });
}
