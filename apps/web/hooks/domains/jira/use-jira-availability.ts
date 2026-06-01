"use client";

import { getJiraConfig } from "@/lib/api/domains/jira-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
} from "../integrations/use-integration-availability";
import { useJiraEnabled } from "./use-jira-enabled";

const fetchJiraConfig = () => getJiraConfig();

export function useJiraAuthed(): boolean {
  return useIntegrationAuthed("jira", fetchJiraConfig);
}

export function useJiraAvailable(): boolean {
  return useIntegrationAvailable({
    kind: "jira",
    useEnabled: useJiraEnabled,
    fetchConfig: fetchJiraConfig,
  });
}
