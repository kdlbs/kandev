"use client";

import { useCallback } from "react";
import { getJiraConfig } from "@/lib/api/domains/jira-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { useJiraEnabled } from "./use-jira-enabled";

export function useJiraAuthed(workspaceId?: string | null): boolean {
  const fetchConfig = useCallback(
    () => getJiraConfig(workspaceId ? { workspaceId } : undefined),
    [workspaceId],
  );
  return useIntegrationAuthed(fetchConfig, { provider: "jira", workspaceId });
}

export function useJiraAvailable(workspaceId?: string | null): boolean {
  const fetchConfig = useCallback(
    () => getJiraConfig(workspaceId ? { workspaceId } : undefined),
    [workspaceId],
  );
  return useIntegrationAvailable({
    provider: "jira",
    workspaceId,
    enabledState: useJiraEnabled(workspaceId),
    fetchConfig,
  });
}
