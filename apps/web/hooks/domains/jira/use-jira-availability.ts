"use client";

import { getJiraConfig } from "@/lib/api/domains/jira-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { qk } from "@/lib/query/keys";
import { useJiraEnabled } from "./use-jira-enabled";

const fetchJiraConfig = () => getJiraConfig();

export function useJiraAuthed(): boolean {
  return useIntegrationAuthed({
    fetchConfig: fetchJiraConfig,
    queryKey: qk.integrations.jira.config(),
  });
}

export function useJiraAvailable(): boolean {
  return useIntegrationAvailable({
    useEnabled: useJiraEnabled,
    fetchConfig: fetchJiraConfig,
    queryKey: qk.integrations.jira.config(),
  });
}
