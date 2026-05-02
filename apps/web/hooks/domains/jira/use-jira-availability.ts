"use client";

import { getJiraConfig } from "@/lib/api/domains/jira-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { useJiraEnabled } from "./use-jira-enabled";

const fetchJiraConfig = () => getJiraConfig();

export function useJiraAuthed(): boolean {
  return useIntegrationAuthed(fetchJiraConfig);
}

export function useJiraAvailable(): boolean {
  return useIntegrationAvailable({
    useEnabled: useJiraEnabled,
    fetchConfig: fetchJiraConfig,
  });
}
