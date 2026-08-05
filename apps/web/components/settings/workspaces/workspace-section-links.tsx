"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { ComponentType } from "react";
import {
  IconArrowsShuffle,
  IconBolt,
  IconGitBranch,
  IconPlugConnected,
} from "@tabler/icons-react";
import { listAutomations } from "@/lib/api/domains/automation-api";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { getAzureDevOpsConfig } from "@/lib/api/domains/azure-devops-api";
import { fetchGitHubStatus } from "@/lib/api/domains/github-auth-api";
import { getGitLabConfig } from "@/lib/api/domains/gitlab-api";
import { getJiraConfig } from "@/lib/api/domains/jira-api";
import { getLinearConfig } from "@/lib/api/domains/linear-api";
import { listSentryInstances } from "@/lib/api/domains/sentry-api";
import { getSlackConfig } from "@/lib/api/domains/slack-api";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import {
  workspaceSettingsHref,
  type WorkspaceSettingsTab,
} from "./workspace-settings-shell";
import { cn } from "@kandev/ui/lib/utils";

type SectionCounts = {
  repositories?: number;
  workflows?: number;
  integrations?: number;
  automations?: number;
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
 */
export function useWorkspaceSectionCounts(workspaceId: string): SectionCounts {
  const [counts, setCounts] = useState<SectionCounts>({});

  useEffect(() => {
    let cancelled = false;
    setCounts({});
    const apply = (patch: SectionCounts) => {
      if (!cancelled) setCounts((prev) => ({ ...prev, ...patch }));
    };
    // The backend serializes empty lists as null — count defensively.
    listRepositories(workspaceId)
      .then((res) => apply({ repositories: (res.repositories ?? []).length }))
      .catch(() => undefined);
    listWorkflows(workspaceId)
      .then((res) => apply({ workflows: (res.workflows ?? []).length }))
      .catch(() => undefined);
    listAutomations(workspaceId)
      .then((automations) => apply({ automations: automations.length }))
      .catch(() => undefined);
    countConfiguredIntegrations(workspaceId)
      .then((integrations) => apply({ integrations }))
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  return counts;
}

type SectionStat = {
  key: keyof SectionCounts;
  tab: WorkspaceSettingsTab;
  labelKey: string;
  icon: ComponentType<{ className?: string }>;
};

const SECTION_STATS: SectionStat[] = [
  { key: "repositories", tab: "repositories", labelKey: "sidebar:repositories", icon: IconGitBranch },
  { key: "workflows", tab: "workflows", labelKey: "workflows:workflows", icon: IconArrowsShuffle },
  { key: "integrations", tab: "integrations", labelKey: "common:integrations", icon: IconPlugConnected },
  { key: "automations", tab: "automations", labelKey: "common:automations", icon: IconBolt },
];

/**
 * Quick links into a workspace's sections with their counts. Rendered above
 * the row's whole-card overlay link (z-10), so both stay clickable without
 * nesting anchors.
 */
export function WorkspaceSectionStats({
  workspaceId,
  className,
}: {
  workspaceId: string;
  className?: string;
}) {
  const { t } = useTranslation();
  const counts = useWorkspaceSectionCounts(workspaceId);

  return (
    <div
      className={cn("flex flex-wrap items-center gap-2", className)}
      data-testid="workspace-section-stats"
    >
      {SECTION_STATS.map(({ key, tab, labelKey, icon: Icon }) => (
        <Button
          key={key}
          asChild
          variant="outline"
          // "lg" is this kit's medium: default is a compact h-7. The outline
          // hover is invisible on dark card backgrounds — brighten it there.
          size="lg"
          className="relative z-10 gap-1.5 hover:border-foreground/30 dark:hover:bg-input/80"
        >
          <Link href={workspaceSettingsHref(workspaceId, tab)}>
            <Icon className="h-4 w-4" />
            {t(labelKey)}
            <span className="font-normal tabular-nums text-muted-foreground/70">
              {counts[key] ?? "–"}
            </span>
          </Link>
        </Button>
      ))}
    </div>
  );
}
