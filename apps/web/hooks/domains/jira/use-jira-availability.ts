"use client";

import { useCallback } from "react";
import { getJiraConfig } from "@/lib/api/domains/jira-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { qk } from "@/lib/query/keys";
import { useJiraEnabled } from "./use-jira-enabled";

export function useJiraAuthed(workspaceId?: string | null): boolean {
  const fetchConfig = useCallback(
    () => getJiraConfig(workspaceId ? { workspaceId } : undefined),
    [workspaceId],
  );
  return useIntegrationAuthed({
    active: workspaceId !== null,
    fetchConfig,
    queryKey: qk.integrations.jira.config(workspaceId),
  });
}

export function useJiraAvailable(workspaceId?: string | null): boolean {
  const fetchConfig = useCallback(
    () => getJiraConfig(workspaceId ? { workspaceId } : undefined),
    [workspaceId],
  );
  return useIntegrationAvailable({
    active: workspaceId !== null,
    useEnabled: useJiraEnabled,
    fetchConfig,
    queryKey: qk.integrations.jira.config(workspaceId),
  });
}
