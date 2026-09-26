"use client";

import { useAppStore } from "@/components/state-provider";
import { useAzureDevOpsAvailable } from "@/hooks/domains/azure-devops/use-azure-devops-availability";
import { useAzureDevOpsEnabled } from "@/hooks/domains/azure-devops/use-azure-devops-enabled";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useGitHubEnabled } from "@/hooks/domains/github/use-github-enabled";
import { useGitLabAvailable } from "@/hooks/domains/gitlab/use-task-mr";
import { useGitLabEnabled } from "@/hooks/domains/gitlab/use-gitlab-enabled";
import { useJiraAuthed } from "@/hooks/domains/jira/use-jira-availability";
import { useJiraEnabled } from "@/hooks/domains/jira/use-jira-enabled";
import { useLinearAuthed } from "@/hooks/domains/linear/use-linear-availability";
import { useLinearEnabled } from "@/hooks/domains/linear/use-linear-enabled";
import { useHideDisabledIntegrationsInNav } from "@/hooks/domains/integrations/use-hide-disabled-integrations-in-nav";
import type { AvailabilityMap } from "@/lib/navigation/types";
import type { GitHubStatus } from "@/lib/types/github";
import { t } from "@/lib/i18n";

/**
 * The workspace the backend resolves when a request names none: the oldest by
 * creation time. Ties (or an empty list) give null, which reads the unscoped
 * toggle exactly as before.
 */
function oldestWorkspace(items: ReadonlyArray<{ id: string; created_at?: string }>): string | null {
  let oldest: { id: string; created_at?: string } | null = null;
  for (const item of items) {
    if (!oldest || (item.created_at ?? "") < (oldest.created_at ?? "")) oldest = item;
  }
  return oldest?.id ?? null;
}

/** Human-readable status label for a not-yet-connected integration destination. */
function getStatusLabel(loading: boolean | undefined): string {
  return loading ? t("common:checking") : t("task:setup");
}

/** Derives the GitHub nav destination's ready/label pair from its auth status. */
export function getGitHubIntegrationStatus(status: GitHubStatus | null, loading: boolean) {
  if (status?.authenticated) return { ready: true, label: t("common:connected") };
  if (status?.token_configured) return { ready: true, label: t("common:configured") };
  return { ready: false, label: getStatusLabel(loading) };
}

/**
 * Availability map the navigation manifest filters on
 * (`lib/navigation/`) — currently which integrations are
 * configured. Extend here when a destination gains a gate.
 *
 * Extracted from `useConfiguredIntegrationLinks` so the sidebar link list and the
 * mobile menu gate on one implementation instead of each calling the five domain
 * hooks themselves.
 *
 * A destination is visible when it is **configured** (credentials saved,
 * auth healthy) AND, only when the user has turned on "Hide disabled
 * integrations from left panel navigation" (`useHideDisabledIntegrationsInNav`,
 * off by default), also **enabled**. With that setting off — the default — a
 * disabled-but-configured integration still shows in the nav exactly like an
 * enabled one; only credential/health status gates it. This deliberately
 * decouples nav visibility from the per-integration "enabled" toggle used
 * elsewhere (import popovers, Kanban external-link buttons, task-top-bar
 * buttons) — those keep gating on enabled-and-authed via each integration's
 * `useXAvailable`, unaffected by this hook. See
 * `docs/specs/integrations/requirements/enable-disable-toggle.md`.
 *
 * Availability is **not** deduplicated across consumers. GitHub and GitLab are
 * store-backed, but Jira, Linear and Azure DevOps go through
 * `useIntegrationAuthed`, which owns a fetch plus a 90s `setInterval` per mounted
 * consumer (`hooks/domains/integrations/use-integration-availability.ts`). So each
 * mounted caller of this hook may add its own polling subscription: keep the
 * callers to the surfaces that render gated sections, and use
 * `useStaticDestinations` everywhere else. Deduplicating this properly means
 * hoisting the probes into the store, which is a change to those domain hooks
 * rather than to the navigation manifest.
 *
 * Not to be confused with `hooks/domains/integrations/use-integration-availability`,
 * which is the generic per-integration primitive these hooks are built on.
 */
export function useNavAvailability(): AvailabilityMap {
  // Jira and Linear are per-workspace integrations, so their availability must
  // be checked against the active workspace. Omitting the id makes the backend
  // fall back to a legacy default-workspace resolver that can point at the
  // wrong workspace, hiding a configured integration from the sidebar. GitHub,
  // Jira, and Linear all follow the active workspace; GitLab remains install-wide.
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const activeWorkspaceExists = useAppStore((s) =>
    s.workspaces.items.some((item) => item.id === s.workspaces.activeId),
  );
  // Guard against a stale active id: if the active workspace was removed but
  // activeId was not reconciled (e.g. setWorkspaces keeps a non-null id),
  // scoping to the deleted id would return no config and hide the links even
  // when another workspace is configured. Fall back to null so the backend's
  // default-workspace resolution applies instead.
  const scopedWorkspaceId = activeWorkspaceExists ? activeWorkspaceId : null;
  // The toggles are browser-local, so a null id has no backend to resolve it:
  // it would read the unscoped key while the probes above answer for whichever
  // workspace the backend picked, and the two could then disagree about the
  // same integration. Mirror the backend's tie-breaker instead
  // (`workspacescope.ResolveMigrationTarget`: oldest workspace by creation),
  // which with the usual single surviving workspace is exactly it.
  const oldestWorkspaceId = useAppStore((s) => oldestWorkspace(s.workspaces.items));
  const toggleWorkspaceId = scopedWorkspaceId ?? oldestWorkspaceId;
  const { status, loading } = useGitHubStatus();
  const gitlabConfigured = useGitLabAvailable();
  const jiraConfigured = useJiraAuthed(scopedWorkspaceId);
  const linearConfigured = useLinearAuthed(scopedWorkspaceId);
  const azureDevOpsConfigured = useAzureDevOpsAvailable(scopedWorkspaceId);
  const githubConfigured = getGitHubIntegrationStatus(status, loading).ready;

  // The toggles are per-workspace too: a workspace that has GitHub turned off
  // must not hide it from the nav while another workspace is active.
  const { enabled: azureDevOpsEnabled } = useAzureDevOpsEnabled(toggleWorkspaceId);
  const { enabled: githubEnabled } = useGitHubEnabled(toggleWorkspaceId);
  const { enabled: gitlabEnabled } = useGitLabEnabled(toggleWorkspaceId);
  const { enabled: jiraEnabled } = useJiraEnabled(toggleWorkspaceId);
  const { enabled: linearEnabled } = useLinearEnabled(toggleWorkspaceId);

  const { hideDisabled } = useHideDisabledIntegrationsInNav();
  const visible = (configured: boolean, enabled: boolean) =>
    configured && (!hideDisabled || enabled);

  return {
    "azure-devops": visible(!!azureDevOpsConfigured, azureDevOpsEnabled),
    github: visible(githubConfigured, githubEnabled),
    gitlab: visible(gitlabConfigured, gitlabEnabled),
    jira: visible(jiraConfigured, jiraEnabled),
    linear: visible(linearConfigured, linearEnabled),
  };
}
