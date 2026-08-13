"use client";

import { useState } from "react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { IconLoader2, IconLock, IconPlus, IconSettings } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { NotInstalledBadge } from "@/components/settings/record-badges";
import { Button } from "@kandev/ui/button";
import { Card } from "@kandev/ui/card";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { AgentLogo } from "@/components/agent-logo";
import { AgentLoginDialog } from "@/components/settings/agent-login-dialog";
import { AgentRuntimeUpdateControl } from "@/components/settings/agent-runtime-update-control";
import { HostShellDialog } from "@/components/settings/host-shell-dialog";
import type { AgentUpdateJob, AgentUpdatePreview, InstallJob } from "@/lib/api";
import type { Agent, AgentDiscovery, RuntimeUpdate } from "@/lib/types/http";

type Props = {
  agent: AgentDiscovery;
  savedAgent: Agent | undefined;
  displayName: string;
  /** Capability status from the host utility probe ("ok", "auth_required", etc.). */
  capabilityStatus?: string;
  runtimeUpdate?: RuntimeUpdate;
  updateJob?: AgentUpdateJob;
  installJob?: InstallJob;
  onPreview?: (agentName: string) => Promise<AgentUpdatePreview>;
  onUpdate?: (agentName: string) => Promise<AgentUpdateJob>;
  /**
   * Called when the auth/shell dialog closes so the page can refresh
   * discovery + availability. Without this the yellow lock stays put even
   * after a successful sign-in, making the recovery flow look broken.
   */
  onAuthComplete?: () => void;
  /** The agent's profiles sub-list, rendered inside the group card. */
  children?: ReactNode;
};

function InstalledAgentIdentity({
  agent,
  displayName,
  configured,
  probing,
  authRequired,
  loginAvailable,
  onAuthClick,
}: {
  agent: AgentDiscovery;
  displayName: string;
  configured: boolean;
  probing: boolean;
  authRequired: boolean;
  loginAvailable: boolean;
  onAuthClick: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <AgentLogo agentName={agent.name} size={20} className="shrink-0" />
        <h4 className="min-w-0 truncate text-lg font-semibold">{displayName}</h4>
        {agent.supports_mcp && <Badge variant="secondary">MCP</Badge>}
        {configured && <Badge variant="outline">{t("agents:configured")}</Badge>}
        {/* Its profiles are still listed and editable below, so the card has to
            say why none of them can run. */}
        {!agent.available && <NotInstalledBadge />}
        <div className="ml-auto flex shrink-0 items-center gap-1">
          {probing && (
            <Tooltip>
              <TooltipTrigger asChild>
                <span
                  data-testid={`probing-icon-${agent.name}`}
                  className="flex items-center text-muted-foreground cursor-help"
                  aria-label={t("agents:checkingAuthentication")}
                >
                  <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
                </span>
              </TooltipTrigger>
              <TooltipContent>{t("agents:checkingCapabilitiesAndAuth")}</TooltipContent>
            </Tooltip>
          )}
          {authRequired && (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  onClick={onAuthClick}
                  data-testid={`auth-icon-${agent.name}`}
                  className="flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs text-amber-500 cursor-pointer hover:bg-amber-500/10"
                  aria-label={t("agents:authenticationRequired")}
                >
                  <IconLock className="h-3.5 w-3.5" />
                </button>
              </TooltipTrigger>
              <TooltipContent>
                {loginAvailable
                  ? t("agents:authRequiredOpenLoginTerminal")
                  : t("agents:authRequiredOpenShell")}
              </TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>
      <p
        className="text-xs text-muted-foreground line-clamp-2"
        title={agent.matched_path ?? undefined}
      >
        {agent.matched_path ? t("agents:detectedAt", { path: agent.matched_path }) : ""}
      </p>
    </div>
  );
}

/**
 * Card rendered under "Installed Agents" - links to the agent's page (its
 * profile list) once configured, or straight into profile creation otherwise.
 * Surfaces a yellow lock icon when the capability probe reports
 * `auth_required`. Clicking the lock opens a PTY login dialog if the agent
 * type has a registered LoginCommand.
 */
export function InstalledAgentCard({
  agent,
  savedAgent,
  displayName,
  capabilityStatus,
  runtimeUpdate,
  updateJob,
  installJob,
  onPreview,
  onUpdate,
  onAuthComplete,
  children,
}: Props) {
  const { t } = useTranslation();
  const configured = Boolean(savedAgent && savedAgent.profiles.length > 0);
  const hasAgentRecord = Boolean(savedAgent);
  const agentHref = `/settings/agents/${encodeURIComponent(agent.name)}`;
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
    <Card className="min-w-0 gap-0 py-0" data-testid={`agent-group-${agent.name}`}>
      {/* Header section: identity + agent-level actions. */}
      <div
        className="flex min-w-0 flex-wrap items-start justify-between gap-3 px-3 py-2.5"
        data-testid={`agent-card-header-${agent.name}`}
      >
        <div className="min-w-0 flex-1">
          <InstalledAgentIdentity
            agent={agent}
            displayName={displayName}
            configured={configured}
            probing={probing}
            authRequired={authRequired}
            loginAvailable={loginAvailable}
            onAuthClick={handleAuthClick}
          />
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {runtimeUpdate?.supported && onPreview && onUpdate && (
            <AgentRuntimeUpdateControl
              agentName={agent.name}
              displayName={displayName}
              runtimeUpdate={runtimeUpdate}
              job={updateJob}
              installJob={installJob}
              onPreview={onPreview}
              onUpdate={onUpdate}
            />
          )}
          {configured ? (
            <Button className="h-11 cursor-pointer md:h-7" asChild>
              <Link href={`${agentHref}?mode=create`} data-testid={`new-profile-${agent.name}`}>
                <IconPlus className="mr-2 h-4 w-4" />
                {t("agents:newProfile")}
              </Link>
            </Button>
          ) : (
            // Keep the setup action for agents without a usable profile. A
            // saved record needs create mode so its first profile is drafted.
            <Button className="h-11 cursor-pointer md:h-7" asChild>
              <Link
                href={hasAgentRecord ? `${agentHref}?mode=create` : agentHref}
                data-testid={`setup-profile-${agent.name}`}
              >
                <IconSettings className="mr-2 h-4 w-4" />
                {t("agents:setupProfile")}
              </Link>
            </Button>
          )}
        </div>
      </div>
      {/* Profiles area: full-bleed below the header, split off by a 1px border. */}
      {children}
      <AuthDialogs
        agent={agent}
        loginOpen={loginOpen}
        setLoginOpen={setLoginOpen}
        shellOpen={shellOpen}
        setShellOpen={setShellOpen}
        loginAvailable={loginAvailable}
        onAuthComplete={onAuthComplete}
      />
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
