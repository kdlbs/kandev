"use client";

import { useMemo } from "react";

import { useAzureDevOpsAvailable } from "@/hooks/domains/azure-devops/use-azure-devops-availability";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useGitLabStatus } from "@/hooks/domains/gitlab/use-gitlab-status";
import { useJiraAuthed } from "@/hooks/domains/jira/use-jira-availability";
import { useLinearAuthed } from "@/hooks/domains/linear/use-linear-availability";
import { useSentryAvailable } from "@/hooks/domains/sentry/use-sentry-availability";
import { WORKSPACE_INTEGRATIONS } from "@/lib/settings-discovery/catalog/integrations";
import {
  INTEGRATION_ENABLED_KEYS,
  INTEGRATION_ENABLED_SYNC_EVENTS,
} from "@/lib/integrations/integration-enabled-keys";
import { useIntegrationEnabledReader } from "./use-integration-enabled";

export type IntegrationSlug = (typeof WORKSPACE_INTEGRATIONS)[number][0];

/**
 * Which integrations are connected and enabled for this workspace. Badges
 * consume saved preferences; settings drafts do not change them until Save.
 */
export function useEnabledIntegrations(workspaceId: string): ReadonlySet<IntegrationSlug> {
  const azureDevOps = useAzureDevOpsAvailable(workspaceId);
  const { status: githubStatus } = useGitHubStatus(workspaceId);
  const { status: gitlabStatus } = useGitLabStatus(workspaceId);
  const readEnabled = useIntegrationEnabledReader(INTEGRATION_ENABLED_SYNC_EVENTS);
  const jira = useJiraAuthed(workspaceId);
  const linear = useLinearAuthed(workspaceId);
  const sentry = useSentryAvailable(workspaceId);
  // GitHub reports a device-flow login and a configured PAT separately; either
  // one is a working connection.
  const github = Boolean(githubStatus?.authenticated || githubStatus?.token_configured);
  const gitlab = Boolean(gitlabStatus?.authenticated || gitlabStatus?.token_configured);

  return useMemo(() => {
    // A `Record` over the slug union rather than a list of spreads: adding an
    // integration to WORKSPACE_INTEGRATIONS then fails to compile here until it
    // declares how it is probed, instead of silently never showing a badge.
    const connected: Record<IntegrationSlug, boolean> = {
      "azure-devops": azureDevOps,
      github,
      gitlab,
      jira,
      linear,
      sentry,
    };
    return new Set(
      WORKSPACE_INTEGRATIONS.map(([slug]) => slug).filter(
        (slug) => connected[slug] && readEnabled(INTEGRATION_ENABLED_KEYS[slug], workspaceId),
      ),
    );
  }, [azureDevOps, github, gitlab, jira, linear, sentry, readEnabled, workspaceId]);
}
