"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronRight } from "@tabler/icons-react";
import { listAutomations } from "@/lib/api/domains/automation-api";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { listSecrets } from "@/lib/api/domains/secrets-api";
import { getAzureDevOpsConfig } from "@/lib/api/domains/azure-devops-api";
import { fetchGitHubStatus } from "@/lib/api/domains/github-auth-api";
import { getGitLabConfig } from "@/lib/api/domains/gitlab-api";
import { getJiraConfig } from "@/lib/api/domains/jira-api";
import { getLinearConfig } from "@/lib/api/domains/linear-api";
import { listSentryInstances } from "@/lib/api/domains/sentry-api";
import { getSlackConfig } from "@/lib/api/domains/slack-api";
import Link from "@/components/routing/app-link";
import {
  workspaceSettingsHref,
  workspaceSettingsTabSpec,
  type WorkspaceSettingsTab,
} from "./workspace-settings-shell";
import { cn } from "@kandev/ui/lib/utils";

export type SectionCounts = {
  repositories?: number;
  workflows?: number;
  integrations?: number;
  automations?: number;
  secrets?: number;
};

// One probe per integration service; counts the ones configured/connected for
// this workspace. There is no aggregate endpoint, but the probes are tiny
// config GETs and workspaces are few.
async function countConfiguredIntegrations(workspaceId: string): Promise<number> {
  const probes: Array<Promise<boolean>> = [
    getAzureDevOpsConfig(workspaceId).then((config) => config !== null),
    fetchGitHubStatus(workspaceId).then((status) => !!status.authenticated),
    getGitLabConfig({ workspaceId }).then((config) => config !== null),
    getJiraConfig({ workspaceId }).then((config) => config !== null),
    getLinearConfig({ workspaceId }).then((config) => config !== null),
    listSentryInstances(workspaceId).then((instances) => instances.length > 0),
    getSlackConfig({ workspaceId }).then((config) => config !== null),
  ];
  const results = await Promise.allSettled(probes);
  return results.filter((result) => result.status === "fulfilled" && result.value).length;
}

/**
 * Lazy per-workspace counts for the list page's rows. Fetched client-side per
 * row: workspaces are few and the lists are small, so a burst of parallel
 * requests per row beats a dedicated summary endpoint for now. Each count
 * lands as soon as its request resolves — one slow endpoint (the integration
 * probes can wait on external services) must not hold the others hostage.
 * `settled` flips once every probe finished, fulfilled or not.
 */
export function useWorkspaceSectionCounts(workspaceId: string): {
  counts: SectionCounts;
  settled: boolean;
} {
  const [counts, setCounts] = useState<SectionCounts>({});
  const [settled, setSettled] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setCounts({});
    setSettled(false);
    const apply = (patch: SectionCounts) => {
      if (!cancelled) setCounts((prev) => ({ ...prev, ...patch }));
    };
    // The backend serializes empty lists as null — count defensively.
    const probes = [
      listRepositories(workspaceId)
        .then((res) => apply({ repositories: (res.repositories ?? []).length }))
        .catch(() => undefined),
      listWorkflows(workspaceId)
        .then((res) => apply({ workflows: (res.workflows ?? []).length }))
        .catch(() => undefined),
      listAutomations(workspaceId)
        .then((automations) => apply({ automations: (automations ?? []).length }))
        .catch(() => undefined),
      countConfiguredIntegrations(workspaceId)
        .then((integrations) => apply({ integrations }))
        .catch(() => undefined),
      listSecrets({ scope: "workspace", workspaceId })
        .then((secrets) => apply({ secrets: (secrets ?? []).length }))
        .catch(() => undefined),
    ];
    void Promise.allSettled(probes).then(() => {
      if (!cancelled) setSettled(true);
    });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  return { counts, settled };
}

type SectionStat = {
  key: keyof SectionCounts;
  tab: WorkspaceSettingsTab;
};

// Name and mark come from the tab table, so a tile and the tab it opens cannot
// end up labelled or marked differently.
const SECTION_STATS: SectionStat[] = [
  { key: "repositories", tab: "repositories" },
  { key: "workflows", tab: "workflows" },
  { key: "integrations", tab: "integrations" },
  { key: "automations", tab: "automations" },
  { key: "secrets", tab: "secrets" },
];

/**
 * A workspace's sections as count tiles: big count, chevron, label. Rendered
 * above the row's whole-card overlay link (z-10), so both stay clickable
 * without nesting anchors. Zero counts dim; unknown counts show an em dash.
 */
export function WorkspaceSectionStats({
  workspaceId,
  counts,
  className,
}: {
  workspaceId: string;
  counts: SectionCounts;
  className?: string;
}) {
  const { t } = useTranslation();

  return (
    <div
      className={cn(
        // Desktop columns cap at 175px per tile instead of stretching full width.
        "grid flex-1 grid-cols-3 gap-2 lg:grid-cols-[repeat(5,minmax(0,175px))]",
        className,
      )}
      data-testid="workspace-section-stats"
    >
      {SECTION_STATS.map(({ key, tab }) => {
        const count = counts[key];
        const { labelKey, icon: Icon } = workspaceSettingsTabSpec(tab);
        return (
          <Link
            key={key}
            href={workspaceSettingsHref(workspaceId, tab)}
            className="relative z-10 flex flex-col gap-1 rounded-lg border border-border/70 bg-background/50 p-2.5 transition-colors hover:border-foreground/30 hover:bg-muted/50"
          >
            <div className="flex items-start justify-between gap-1">
              <span
                className={cn(
                  "text-lg font-bold leading-none tabular-nums",
                  (count === 0 || count === undefined) && "text-muted-foreground/50",
                )}
              >
                {count ?? "—"}
              </span>
              <IconChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground/50" />
            </div>
            {/* gap-1, not gap-1.5: at the width these tiles sit at on a laptop
                the longest label ("Automations") overflowed its box by a single
                pixel once the mark took its place beside it. */}
            <span className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
              <Icon className="h-3.5 w-3.5 shrink-0" />
              <span className="truncate">{t(labelKey)}</span>
            </span>
          </Link>
        );
      })}
    </div>
  );
}
