"use client";

import Link from "next/link";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconBrandGithub, IconHexagon, IconTicket } from "@tabler/icons-react";
import { useJiraAvailable } from "@/components/jira/my-jira/use-jira-availability";
import { useLinearAvailable } from "@/components/linear/use-linear-availability";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import type { GitHubStatus } from "@/lib/types/github";

type IntegrationsProps = {
  workspaceId?: string;
};

type MobileIntegrationsSectionProps = IntegrationsProps & {
  onNavigate: () => void;
};

type IntegrationId = "github" | "jira" | "linear";

type IntegrationLink = {
  id: IntegrationId;
  label: string;
  href: string;
};

type IntegrationAvailability = {
  githubReady: boolean;
  jiraAvailable: boolean;
  linearAvailable: boolean;
};

const INTEGRATION_LINKS: IntegrationLink[] = [
  { id: "github", label: "GitHub", href: "/github" },
  { id: "jira", label: "Jira", href: "/jira" },
  { id: "linear", label: "Linear", href: "/linear" },
];

const INTEGRATION_ICONS = {
  github: IconBrandGithub,
  jira: IconTicket,
  linear: IconHexagon,
} satisfies Record<IntegrationId, typeof IconBrandGithub>;

export function getAvailableIntegrationLinks({
  githubReady,
  jiraAvailable,
  linearAvailable,
}: IntegrationAvailability): IntegrationLink[] {
  return INTEGRATION_LINKS.filter((link) => {
    if (link.id === "github") return githubReady;
    if (link.id === "jira") return jiraAvailable;
    return linearAvailable;
  });
}

function getStatusLabel(connected: boolean, loading: boolean | undefined): string {
  if (loading) return "Checking";
  return connected ? "Connected" : "Setup";
}

export function getGitHubIntegrationStatus(status: GitHubStatus | null, loading: boolean) {
  if (status?.authenticated) return { ready: true, label: "Connected" };
  if (status?.token_configured) return { ready: true, label: "Configured" };
  return { ready: false, label: getStatusLabel(false, loading) };
}

function useConfiguredIntegrationLinks(workspaceId: string | undefined): IntegrationLink[] {
  const { status, loading } = useGitHubStatus();
  const jiraAvailable = useJiraAvailable(workspaceId);
  const linearAvailable = useLinearAvailable(workspaceId);
  const githubStatus = getGitHubIntegrationStatus(status, loading);

  return getAvailableIntegrationLinks({
    githubReady: githubStatus.ready,
    jiraAvailable,
    linearAvailable,
  });
}

function DesktopIntegrationButton({ link }: { link: IntegrationLink }) {
  const Icon = INTEGRATION_ICONS[link.id];

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          asChild
          variant="ghost"
          size="lg"
          className="text-muted-foreground hover:text-foreground"
        >
          <Link href={link.href} aria-label={`Open ${link.label}`}>
            <Icon className="h-4 w-4" />
            <span className="hidden xl:inline">{link.label}</span>
          </Link>
        </Button>
      </TooltipTrigger>
      <TooltipContent>Open {link.label}</TooltipContent>
    </Tooltip>
  );
}

export function IntegrationNavButtons({ workspaceId }: IntegrationsProps) {
  const links = useConfiguredIntegrationLinks(workspaceId);

  if (links.length === 0) return null;

  return (
    <nav className="flex items-center gap-1" aria-label="Configured integrations">
      {links.map((link) => (
        <DesktopIntegrationButton key={link.id} link={link} />
      ))}
    </nav>
  );
}

export function MobileIntegrationsSection({
  workspaceId,
  onNavigate,
}: MobileIntegrationsSectionProps) {
  const links = useConfiguredIntegrationLinks(workspaceId);

  if (links.length === 0) return null;

  return (
    <div className="space-y-3">
      <div className="text-sm font-medium">Integrations</div>
      {links.map((link) => {
        const Icon = INTEGRATION_ICONS[link.id];
        return (
          <Button key={link.id} asChild variant="outline" className="w-full justify-start gap-2">
            <Link href={link.href} onClick={onNavigate}>
              <Icon className="h-4 w-4" />
              <span className="flex-1 text-left">{link.label}</span>
            </Link>
          </Button>
        );
      })}
    </div>
  );
}
