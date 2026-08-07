"use client";

import Link from "@/components/routing/app-link";
import {
  IconBrandGithub,
  IconBrandGitlab,
  IconBrandAzure,
  IconBrandSentry,
  IconBrandSlack,
  IconHexagon,
  IconTicket,
} from "@tabler/icons-react";
import { Card, CardContent } from "@kandev/ui/card";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { usePluginRegistry } from "@/lib/plugins/registry";

const INTEGRATIONS = [
  {
    slug: "azure-devops",
    label: "Azure DevOps",
    description: "Azure Boards work items and Azure Repos pull requests.",
    Icon: IconBrandAzure,
  },
  {
    slug: "github",
    label: "GitHub",
    description: "PR review queues, issue watchers, and OAuth credentials.",
    Icon: IconBrandGithub,
  },
  {
    slug: "gitlab",
    label: "GitLab",
    description: "Merge request creation, discussion replies, and self-managed hosts.",
    Icon: IconBrandGitlab,
  },
  {
    slug: "jira",
    label: "Jira",
    description: "Atlassian Cloud credentials and JQL issue watchers.",
    Icon: IconTicket,
  },
  {
    slug: "linear",
    label: "Linear",
    description: "Personal API key and team defaults.",
    Icon: IconHexagon,
  },
  {
    slug: "sentry",
    label: "Sentry",
    description: "Auth token, org/project defaults, and issue browsing.",
    Icon: IconBrandSentry,
  },
  {
    slug: "slack",
    label: "Slack",
    description: "Browser-session credentials and !kandev triage agent.",
    Icon: IconBrandSlack,
  },
];

type IntegrationsIndexPageProps = {
  workspaceId?: string;
};

export default function IntegrationsIndexPage({ workspaceId }: IntegrationsIndexPageProps = {}) {
  const registry = usePluginRegistry();
  const rootHref = workspaceId
    ? `/settings/workspace/${encodeURIComponent(workspaceId)}/integrations`
    : "/settings/integrations";
  const integrations = [
    ...INTEGRATIONS,
    ...registry.getIntegrationSettings().map(({ id, label, description, icon }) => ({
      slug: id,
      label,
      description,
      Icon: resolvePluginIcon(icon),
    })),
  ];

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">Integrations</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Connect Kandev to third-party services. Connection scope and available settings are shown
          on each integration page.
        </p>
      </div>
      <div className="grid auto-rows-fr gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {integrations.map(({ slug, label, description, Icon }) => {
          const href = `${rootHref}/${slug}`;
          return (
            <Link key={href} href={href} className="flex h-full cursor-pointer">
              <Card className="h-full w-full transition-colors hover:border-primary/40">
                <CardContent className="space-y-2">
                  <div className="flex items-center gap-2 text-base font-semibold">
                    <Icon className="h-5 w-5" />
                    {label}
                  </div>
                  <p className="text-sm text-muted-foreground">{description}</p>
                </CardContent>
              </Card>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
