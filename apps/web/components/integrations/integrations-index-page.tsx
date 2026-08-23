"use client";

import type { ComponentType, ReactNode } from "react";
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
import { Separator } from "@kandev/ui/separator";
import { Switch } from "@kandev/ui/switch";
import { WorkspaceSectionHeader } from "@/components/settings/workspaces/workspace-section-header";
import { SettingsPageHeader } from "@/components/settings/settings-typography";
import { useTranslation } from "react-i18next";
import { useDraftedIntegrationEnabled } from "@/components/integrations/use-drafted-integration-enabled";
import { useHideDisabledIntegrationsInNav } from "@/hooks/domains/integrations/use-hide-disabled-integrations-in-nav";
import { AzureDevOpsEnabledControl } from "@/components/azure-devops/azure-devops-enabled-control";
import type { IntegrationEnabledControlProps } from "@/components/integrations/integration-enabled-control-props";
import { GitHubEnabledControl } from "@/components/github/github-enabled-control";
import { GitLabEnabledControl } from "@/components/gitlab/gitlab-enabled-control";
import { JiraEnabledControl } from "@/components/jira/jira-enabled-control";
import { LinearEnabledControl } from "@/components/linear/linear-enabled-control";
import { SentryEnabledControl } from "@/components/sentry/sentry-enabled-control";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { PluginErrorBoundary } from "@/components/plugins/plugin-error-boundary";

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
const ENABLED_CONTROL_BY_SLUG: Record<
  IntegrationSlug,
  ComponentType<IntegrationEnabledControlProps>
> = {
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

type IntegrationCardProps = {
  href: string;
  label: string;
  description: ReactNode;
  Icon: ComponentType<{ className?: string }>;
  control?: ReactNode;
  testId: string;
};

function IntegrationCard({
  href,
  label,
  description,
  Icon,
  control,
  testId,
}: IntegrationCardProps) {
  return (
    <Card
      data-testid={testId}
      className="relative h-full w-full transition-colors hover:border-primary/40"
    >
      <Link
        href={href}
        aria-label={label}
        className="absolute inset-0 rounded-[inherit] cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
      />
      <CardContent className="space-y-2">
        <div className="flex items-center justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2 text-base font-semibold">
            <Icon className="h-5 w-5 shrink-0" />
            <span className="truncate">{label}</span>
          </div>
          {control ? <div className="relative z-10 shrink-0">{control}</div> : null}
        </div>
        <p className="text-sm text-muted-foreground">{description}</p>
      </CardContent>
    </Card>
  );
}

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
  const registry = usePluginRegistry();
  const { t } = useTranslation();
  const rootHref = workspaceId
    ? `/settings/workspaces/${encodeURIComponent(workspaceId)}/integrations`
    : "/settings/integrations";
  const pluginIntegrations = registry.getIntegrationSettings();

  return (
    <div className="space-y-6">
      {workspaceId ? (
        <WorkspaceSectionHeader
          tab="integrations"
          description={t("settings:connectKandevToThirdPartyServices")}
        />
      ) : (
        <SettingsPageHeader
          title={t("common:integrations")}
          description={t("settings:connectKandevToThirdPartyServices")}
        />
      )}
      <Separator />
      <div className="grid auto-rows-fr gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {INTEGRATIONS.map(({ slug, label, descriptionKey, Icon }) => {
          const href = `${rootHref}/${slug}`;
          const EnabledControl = ENABLED_CONTROL_BY_SLUG[slug];
          return (
            <IntegrationCard
              key={slug}
              testId={`integration-card-${slug}`}
              href={href}
              label={label}
              Icon={Icon}
              description={t(descriptionKey)}
              control={<EnabledControl workspaceId={workspaceId} />}
            />
          );
        })}
        {pluginIntegrations.map(({ pluginId, id, label, description, icon, action: Action }) => {
          const href = `${rootHref}/${id}`;
          const Icon = resolvePluginIcon(icon);
          return (
            <IntegrationCard
              key={`${pluginId}:${id}`}
              testId={`integration-card-${pluginId}-${id}`}
              href={href}
              label={label}
              Icon={Icon}
              description={description}
              control={
                Action ? (
                  <PluginErrorBoundary context={`integration card action "${pluginId}:${id}"`}>
                    <Action workspaceId={workspaceId} surface="index" />
                  </PluginErrorBoundary>
                ) : null
              }
            />
          );
        })}
      </div>
      <HideDisabledIntegrationsSetting />
    </div>
  );
}
