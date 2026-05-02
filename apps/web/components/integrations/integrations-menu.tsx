"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { IconBrandGithub, IconHexagon, IconPlugConnected, IconTicket } from "@tabler/icons-react";
import { useJiraAvailable } from "@/hooks/domains/jira/use-jira-availability";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
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

const HOVER_CLOSE_DELAY_MS = 180;

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

function getStatusLabel(loading: boolean | undefined): string {
  return loading ? "Checking" : "Setup";
}

export function getGitHubIntegrationStatus(status: GitHubStatus | null, loading: boolean) {
  if (status?.authenticated) return { ready: true, label: "Connected" };
  if (status?.token_configured) return { ready: true, label: "Configured" };
  return { ready: false, label: getStatusLabel(loading) };
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

export function IntegrationsMenu({ workspaceId }: IntegrationsProps) {
  const links = useConfiguredIntegrationLinks(workspaceId);
  const [open, setOpen] = useState(false);
  const closeTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (closeTimeoutRef.current) clearTimeout(closeTimeoutRef.current);
    };
  }, []);

  const clearCloseTimeout = () => {
    if (!closeTimeoutRef.current) return;
    clearTimeout(closeTimeoutRef.current);
    closeTimeoutRef.current = null;
  };

  const openOnHover = () => {
    clearCloseTimeout();
    setOpen((current) => (current ? current : true));
  };

  const closeAfterHover = () => {
    clearCloseTimeout();
    closeTimeoutRef.current = setTimeout(() => setOpen(false), HOVER_CLOSE_DELAY_MS);
  };

  const handleOpenChange = (nextOpen: boolean) => {
    clearCloseTimeout();
    setOpen(nextOpen);
  };

  if (links.length === 0) return null;

  return (
    <DropdownMenu open={open} onOpenChange={handleOpenChange} modal={false}>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-lg"
          className="cursor-pointer text-muted-foreground hover:text-foreground"
          aria-label="Integrations"
          onPointerEnter={openOnHover}
          onPointerLeave={closeAfterHover}
        >
          <IconPlugConnected className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        className="w-48"
        onPointerEnter={openOnHover}
        onPointerLeave={closeAfterHover}
      >
        <DropdownMenuLabel>Integrations</DropdownMenuLabel>
        {links.map((link) => {
          const Icon = INTEGRATION_ICONS[link.id];
          return (
            <DropdownMenuItem key={link.id} asChild className="cursor-pointer">
              <Link href={link.href}>
                <Icon className="h-4 w-4 text-muted-foreground" />
                {link.label}
              </Link>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
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
          <Button
            key={link.id}
            asChild
            variant="outline"
            className="w-full cursor-pointer justify-start gap-2"
          >
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
