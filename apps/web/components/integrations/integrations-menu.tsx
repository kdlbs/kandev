"use client";

import Link from "next/link";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import {
  IconBrandGithub,
  IconChevronDown,
  IconPlugConnected,
  IconTicket,
} from "@tabler/icons-react";
import { useJiraAvailable } from "@/components/jira/my-jira/use-jira-availability";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import type { GitHubStatus } from "@/lib/types/github";

type IntegrationsProps = {
  workspaceId?: string;
};

type MobileIntegrationsSectionProps = IntegrationsProps & {
  onNavigate: () => void;
};

function getJiraHref(workspaceId: string | undefined, available: boolean): string {
  if (available) return "/jira";
  return workspaceId ? `/settings/workspace/${workspaceId}/jira` : "/settings";
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

function IntegrationStatusBadge({
  connected,
  loading,
  label,
}: {
  connected: boolean;
  loading?: boolean;
  label?: string;
}) {
  const statusLabel = label ?? getStatusLabel(connected, loading);
  const className = connected ? "text-success" : "text-muted-foreground";

  return (
    <Badge variant="secondary" className={className}>
      {statusLabel}
    </Badge>
  );
}

export function IntegrationsMenu({ workspaceId }: IntegrationsProps) {
  const { status, loading } = useGitHubStatus();
  const jiraAvailable = useJiraAvailable(workspaceId);
  const githubStatus = getGitHubIntegrationStatus(status, loading);
  const githubHref = githubStatus.ready ? "/github" : "/settings/general/github";
  const jiraHref = getJiraHref(workspaceId, jiraAvailable);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" className="cursor-pointer">
          <IconPlugConnected className="h-4 w-4" />
          Integrations
          <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel>Integrations</DropdownMenuLabel>
        <DropdownMenuItem asChild className="cursor-pointer">
          <Link href={githubHref} className="gap-3">
            <IconBrandGithub className="h-4 w-4 text-muted-foreground" />
            <span className="flex-1">GitHub</span>
            <IntegrationStatusBadge
              connected={githubStatus.ready}
              loading={loading}
              label={githubStatus.label}
            />
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild className="cursor-pointer">
          <Link href={jiraHref} className="gap-3">
            <IconTicket className="h-4 w-4 text-muted-foreground" />
            <span className="flex-1">Jira</span>
            <IntegrationStatusBadge connected={jiraAvailable} />
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function MobileIntegrationsSection({
  workspaceId,
  onNavigate,
}: MobileIntegrationsSectionProps) {
  const { status, loading } = useGitHubStatus();
  const jiraAvailable = useJiraAvailable(workspaceId);
  const githubStatus = getGitHubIntegrationStatus(status, loading);
  const githubHref = githubStatus.ready ? "/github" : "/settings/general/github";
  const jiraHref = getJiraHref(workspaceId, jiraAvailable);

  return (
    <div className="space-y-3">
      <div className="text-sm font-medium">Integrations</div>
      <Button asChild variant="outline" className="w-full cursor-pointer justify-start gap-2">
        <Link href={githubHref} onClick={onNavigate}>
          <IconBrandGithub className="h-4 w-4" />
          <span className="flex-1 text-left">GitHub</span>
          <IntegrationStatusBadge
            connected={githubStatus.ready}
            loading={loading}
            label={githubStatus.label}
          />
        </Link>
      </Button>
      <Button asChild variant="outline" className="w-full cursor-pointer justify-start gap-2">
        <Link href={jiraHref} onClick={onNavigate}>
          <IconTicket className="h-4 w-4" />
          <span className="flex-1 text-left">Jira</span>
          <IntegrationStatusBadge connected={jiraAvailable} />
        </Link>
      </Button>
    </div>
  );
}
