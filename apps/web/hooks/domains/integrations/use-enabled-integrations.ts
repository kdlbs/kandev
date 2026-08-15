"use client";

import { useMemo } from "react";

import { useAzureDevOpsAvailable } from "@/hooks/domains/azure-devops/use-azure-devops-availability";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useGitLabAvailable } from "@/hooks/domains/gitlab/use-task-mr";
import { useJiraAuthed } from "@/hooks/domains/jira/use-jira-availability";
import { useLinearAuthed } from "@/hooks/domains/linear/use-linear-availability";
import { useSentryAvailable } from "@/hooks/domains/sentry/use-sentry-availability";
import { WORKSPACE_INTEGRATIONS } from "@/lib/settings-discovery/catalog/integrations";

export type IntegrationSlug = (typeof WORKSPACE_INTEGRATIONS)[number][0];

/**
 * Which of a workspace's integrations are actually connected.
 *
 * "Connected" is the backend's verdict, not a local preference: each probe
 * reports true only once credentials are stored **and** the most recent health
 * check passed (`useIntegrationAuthed`). Every probe starts at `false` and
 * resets on a workspace switch, so a badge never appears before its
 * integration has answered and never carries over from another workspace.
 *
 * The predicates are the ones the settings tree used before it flattened into
 * a menu, kept identical so the badge still means what it used to. Note the
 * asymmetry that carries over with them: most read the connection alone, while
 * Sentry's `available` also requires that workspace's enable toggle, so a
 * connected Sentry whose toggle is off reports false.
 */
export function useEnabledIntegrations(workspaceId: string): ReadonlySet<IntegrationSlug> {
  const azureDevOps = useAzureDevOpsAvailable(workspaceId);
  const { status: githubStatus } = useGitHubStatus(workspaceId);
  const gitlab = useGitLabAvailable();
  const jira = useJiraAuthed(workspaceId);
  const linear = useLinearAuthed(workspaceId);
  const sentry = useSentryAvailable(workspaceId);
  // GitHub reports a device-flow login and a configured PAT separately; either
  // one is a working connection.
  const github = Boolean(githubStatus?.authenticated || githubStatus?.token_configured);

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
    return new Set(WORKSPACE_INTEGRATIONS.map(([slug]) => slug).filter((slug) => connected[slug]));
  }, [azureDevOps, github, gitlab, jira, linear, sentry]);
}
