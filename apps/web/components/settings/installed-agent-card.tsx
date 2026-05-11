"use client";

import { useState } from "react";
import Link from "next/link";
import { IconLoader2, IconLock, IconSettings } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { AgentLogo } from "@/components/agent-logo";
import { AgentLoginDialog } from "@/components/settings/agent-login-dialog";
import { HostShellDialog } from "@/components/settings/host-shell-dialog";
import type { Agent, AgentDiscovery } from "@/lib/types/http";

type Props = {
  agent: AgentDiscovery;
  savedAgent: Agent | undefined;
  displayName: string;
  /** Capability status from the host utility probe ("ok", "auth_required", etc.). */
  capabilityStatus?: string;
  /**
   * Called when the auth/shell dialog closes so the page can refresh
   * discovery + availability. Without this the yellow lock stays put even
   * after a successful sign-in, making the recovery flow look broken.
   */
  onAuthComplete?: () => void;
};

/**
 * Card rendered under "Installed Agents" - links to the agent profile
 * editor and surfaces a yellow lock icon when the capability probe reports
 * `auth_required`. Clicking the lock opens a PTY login dialog if the agent
 * type has a registered LoginCommand.
 */
export function InstalledAgentCard({
  agent,
  savedAgent,
  displayName,
  capabilityStatus,
  onAuthComplete,
}: Props) {
  const configured = Boolean(savedAgent && savedAgent.profiles.length > 0);
  const hasAgentRecord = Boolean(savedAgent);
  const [loginOpen, setLoginOpen] = useState(false);
  const [shellOpen, setShellOpen] = useState(false);
  const authRequired = capabilityStatus === "auth_required";
  const probing = capabilityStatus === "probing";
  const loginAvailable = Boolean(agent.login_command);

  // Either we have a registered login command (open the dedicated login PTY)
  // or we don't (open a plain host shell so the user can explore via
  // `<agent> --help`, run their own auth recipe, etc.).
  const handleAuthClick = () => {
    if (loginAvailable) setLoginOpen(true);
    else setShellOpen(true);
  };

  return (
    <Card className="flex flex-col">
      <CardContent className="py-4 flex flex-col gap-3 flex-1">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <AgentLogo agentName={agent.name} size={20} className="shrink-0" />
            <h4 className="font-medium">{displayName}</h4>
            {agent.supports_mcp && <Badge variant="secondary">MCP</Badge>}
            {configured && <Badge variant="outline">Configured</Badge>}
            {probing && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span
                    data-testid={`probing-icon-${agent.name}`}
                    className="ml-auto flex items-center text-muted-foreground cursor-help"
                    aria-label="Checking authentication"
                  >
                    <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
                  </span>
                </TooltipTrigger>
                <TooltipContent>Checking agent capabilities and authentication...</TooltipContent>
              </Tooltip>
            )}
            {authRequired && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    onClick={handleAuthClick}
                    data-testid={`auth-icon-${agent.name}`}
                    className="ml-auto flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs text-amber-500 cursor-pointer hover:bg-amber-500/10"
                    aria-label="Authentication required"
                  >
                    <IconLock className="h-3.5 w-3.5" />
                  </button>
                </TooltipTrigger>
                <TooltipContent>
                  {loginAvailable
                    ? "Authentication required - click to open login terminal"
                    : "Authentication required - click to open a shell and sign in"}
                </TooltipContent>
              </Tooltip>
            )}
          </div>
          {/* Reserve two lines so cards align regardless of path length. */}
          <p
            className="text-xs text-muted-foreground line-clamp-2 min-h-[2rem]"
            title={agent.matched_path ?? undefined}
          >
            {agent.matched_path ? `Detected at ${agent.matched_path}` : ""}
          </p>
        </div>
        <Button size="sm" className="cursor-pointer mt-auto" asChild>
          <Link
            href={
              hasAgentRecord
                ? `/settings/agents/${encodeURIComponent(agent.name)}?mode=create`
                : `/settings/agents/${encodeURIComponent(agent.name)}`
            }
          >
            <IconSettings className="h-4 w-4 mr-2" />
            {hasAgentRecord ? "Create new profile" : "Setup Profile"}
          </Link>
        </Button>
        <AuthDialogs
          agent={agent}
          loginOpen={loginOpen}
          setLoginOpen={setLoginOpen}
          shellOpen={shellOpen}
          setShellOpen={setShellOpen}
          loginAvailable={loginAvailable}
          onAuthComplete={onAuthComplete}
        />
      </CardContent>
    </Card>
  );
}

function AuthDialogs({
  agent,
  loginOpen,
  setLoginOpen,
  shellOpen,
  setShellOpen,
  loginAvailable,
  onAuthComplete,
}: {
  agent: AgentDiscovery;
  loginOpen: boolean;
  setLoginOpen: (open: boolean) => void;
  shellOpen: boolean;
  setShellOpen: (open: boolean) => void;
  loginAvailable: boolean;
  onAuthComplete?: () => void;
}) {
  if (loginAvailable) {
    return (
      <AgentLoginDialog
        open={loginOpen}
        onOpenChange={setLoginOpen}
        agentName={agent.name}
        description={agent.login_command?.description}
        command={agent.login_command?.cmd}
        onLoginSuccess={onAuthComplete}
      />
    );
  }
  return <HostShellDialog open={shellOpen} onOpenChange={setShellOpen} onClose={onAuthComplete} />;
}
