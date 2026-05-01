"use client";

import { getJiraConfig } from "@/lib/api/domains/jira-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { useJiraEnabled } from "./use-jira-enabled";

export function useJiraAuthed(workspaceId: string | undefined): boolean {
  return useIntegrationAuthed(workspaceId, getJiraConfig);
}

export function useJiraAvailable(workspaceId: string | null | undefined): boolean {
  return useIntegrationAvailable(workspaceId, {
    useEnabled: useJiraEnabled,
    fetchConfig: getJiraConfig,
  });
}
