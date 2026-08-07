"use client";

import type { ComponentType } from "react";
import Link from "@/components/routing/app-link";
import {
  IconBrandGithub,
  IconBrandGitlab,
  IconBrandAzure,
  IconBrandSentry,
  IconHexagon,
  IconTicket,
} from "@tabler/icons-react";
import { Card, CardContent } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useTranslation } from "react-i18next";
import { useDraftedIntegrationEnabled } from "@/components/integrations/use-drafted-integration-enabled";
import { useHideDisabledIntegrationsInNav } from "@/hooks/domains/integrations/use-hide-disabled-integrations-in-nav";
import { AzureDevOpsEnabledControl } from "@/components/azure-devops/azure-devops-enabled-control";
import { GitHubEnabledControl } from "@/components/github/github-enabled-control";
import { GitLabEnabledControl } from "@/components/gitlab/gitlab-enabled-control";
import { JiraEnabledControl } from "@/components/jira/jira-enabled-control";
import { LinearEnabledControl } from "@/components/linear/linear-enabled-control";
import { SentryEnabledControl } from "@/components/sentry/sentry-enabled-control";

type IntegrationSlug = "azure-devops" | "github" | "gitlab" | "jira" | "linear" | "sentry";

const INTEGRATIONS: Array<{
  slug: IntegrationSlug;
  label: string;
  descriptionKey: string;
  Icon: ComponentType<{ className?: string }>;
}> = [
  {
    slug: "azure-devops",
    label: "Azure DevOps",
    descriptionKey: "settings:integrationDescriptionAzureDevops",
    Icon: IconBrandAzure,
  },
  {
    slug: "github",
    label: "GitHub",
    descriptionKey: "settings:integrationDescriptionGithub",
    Icon: IconBrandGithub,
  },
  {
    slug: "gitlab",
    label: "GitLab",
    descriptionKey: "settings:integrationDescriptionGitlab",
    Icon: IconBrandGitlab,
  },
  {
    slug: "jira",
    label: "Jira",
    descriptionKey: "settings:integrationDescriptionJira",
    Icon: IconTicket,
  },
  {
    slug: "linear",
    label: "Linear",
    descriptionKey: "settings:integrationDescriptionLinear",
    Icon: IconHexagon,
  },
  {
    slug: "sentry",
    label: "Sentry",
    descriptionKey: "settings:integrationDescriptionSentry",
    Icon: IconBrandSentry,
  },
];

// Each row's slider is a per-integration hook wrapper (rules of hooks forbid
// picking a hook dynamically by slug), so the map below selects the right
// *component* — every component calls exactly one hook unconditionally.
const ENABLED_CONTROL_BY_SLUG: Record<IntegrationSlug, ComponentType> = {
  "azure-devops": AzureDevOpsEnabledControl,
  github: GitHubEnabledControl,
  gitlab: GitLabEnabledControl,
  jira: JiraEnabledControl,
  linear: LinearEnabledControl,
  sentry: SentryEnabledControl,
};

type IntegrationsIndexPageProps = {
  workspaceId?: string;
};

/** Row for the "Hide disabled integrations from left panel navigation" setting, drafted/saved like the per-integration toggles. */
function HideDisabledIntegrationsSetting() {
  const { t } = useTranslation();
  const { hideDisabled, setHideDisabled } = useHideDisabledIntegrationsInNav();
  const draft = useDraftedIntegrationEnabled({
    id: "integrations-hide-disabled-in-nav",
    enabled: hideDisabled,
    persist: setHideDisabled,
  });
  return (
    <div className="flex min-h-11 items-center justify-between gap-4 rounded-lg border p-4">
      <div className="min-w-0 space-y-0.5">
        <Label htmlFor="hide-disabled-integrations-in-nav">
          {t("settings:hideDisabledIntegrationsFromNav")}
        </Label>
        <p className="text-xs text-muted-foreground">
          {t("settings:hideDisabledIntegrationsFromNavDescription")}
        </p>
      </div>
      <Switch
        id="hide-disabled-integrations-in-nav"
        checked={draft.enabled}
        data-settings-dirty={draft.isDirty}
        onCheckedChange={draft.setEnabled}
        className="shrink-0 cursor-pointer"
      />
    </div>
  );
}

/** `/settings/integrations` and its workspace-scoped equivalent. */
export function IntegrationsIndexPage({ workspaceId }: IntegrationsIndexPageProps = {}) {
  const { t } = useTranslation();
  const rootHref = workspaceId
    ? `/settings/workspace/${encodeURIComponent(workspaceId)}/integrations`
    : "/settings/integrations";

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">{t("common:integrations")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("settings:connectKandevToThirdPartyServices")}
        </p>
      </div>
      <div className="grid auto-rows-fr gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {INTEGRATIONS.map(({ slug, label, descriptionKey, Icon }) => {
          const href = `${rootHref}/${slug}`;
          const EnabledControl = ENABLED_CONTROL_BY_SLUG[slug];
          return (
            <Card
              key={slug}
              data-testid={`integration-card-${slug}`}
              className="h-full w-full transition-colors hover:border-primary/40"
            >
              <CardContent className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <Link
                    href={href}
                    className="flex min-w-0 items-center gap-2 text-base font-semibold hover:underline cursor-pointer"
                  >
                    <Icon className="h-5 w-5 shrink-0" />
                    <span className="truncate">{label}</span>
                  </Link>
                  <div className="shrink-0">
                    <EnabledControl />
                  </div>
                </div>
                <Link href={href} className="text-sm text-muted-foreground cursor-pointer">
                  {t(descriptionKey)}
                </Link>
              </CardContent>
            </Card>
          );
        })}
      </div>
      <HideDisabledIntegrationsSetting />
    </div>
  );
}
