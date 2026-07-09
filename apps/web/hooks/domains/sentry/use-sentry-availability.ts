"use client";

import { useCallback } from "react";
import { fetchSentryConfig } from "@/lib/api/domains/sentry-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { useSentryEnabled } from "./use-sentry-enabled";

export function useSentryAuthed(workspaceId?: string | null): boolean {
  const fetchConfig = useCallback(
    async () => (await fetchSentryConfig(workspaceId ? { workspaceId } : undefined)) ?? null,
    [workspaceId],
  );
  return useIntegrationAuthed(fetchConfig);
}

export function useSentryAvailable(workspaceId?: string | null): boolean {
  const fetchConfig = useCallback(
    async () => (await fetchSentryConfig(workspaceId ? { workspaceId } : undefined)) ?? null,
    [workspaceId],
  );
  return useIntegrationAvailable({
    useEnabled: useSentryEnabled,
    fetchConfig,
  });
}
