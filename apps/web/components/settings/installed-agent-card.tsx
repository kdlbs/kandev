"use client";

import { useState } from "react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import {
  IconChevronDown,
  IconLoader2,
  IconLock,
  IconPlus,
  IconSettings,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { NotInstalledBadge } from "@/components/settings/record-badges";
import { Button } from "@kandev/ui/button";
import { Card } from "@kandev/ui/card";
import { Collapsible, CollapsibleContent } from "@kandev/ui/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useCollapsedAgentBlocks } from "@/hooks/domains/settings/use-collapsed-agent-blocks";
import { AgentLogo } from "@/components/agent-logo";
import { AgentLoginDialog } from "@/components/settings/agent-login-dialog";
import { AgentRuntimeUpdateControl } from "@/components/settings/agent-runtime-update-control";
import { HostShellDialog } from "@/components/settings/host-shell-dialog";
import type { AgentUpdateJob, AgentUpdatePreview, AgentUpdateStatus, InstallJob } from "@/lib/api";
import type { Agent, AgentDiscovery, RuntimeUpdate } from "@/lib/types/http";

type Props = {
  agent: AgentDiscovery;
  savedAgent: Agent | undefined;
  displayName: string;
  /** Capability status from the host utility probe ("ok", "auth_required", etc.). */
  capabilityStatus?: string;
  runtimeUpdate?: RuntimeUpdate;
  runtimeUpdateStatus?: AgentUpdateStatus;
  updateJob?: AgentUpdateJob;
  installJob?: InstallJob;
  onPreview?: (
    agentName: string,
    targetVersion?: string,
    useDefault?: boolean,
  ) => Promise<AgentUpdatePreview>;
  onUpdate?: (
    agentName: string,
    targetVersion: string,
    useDefault?: boolean,
  ) => Promise<AgentUpdateJob>;
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
 * The header's collapse toggle. Sized and styled exactly like the runtime
 * update trigger it sits beside (`ghost` + `icon`, grey only on hover, 44px on
 * touch, 28px from `sm` up) so the action cluster reads as one row. The
 * profiles body is a `CollapsibleContent` driven by this button (no Radix
 * trigger — the trigger would live outside the root, so the button owns the
 * state transition).
 */
function AgentCollapseControl({
  agentName,
  displayName,
  isCollapsed,
  onToggle,
}: {
  agentName: string;
  displayName: string;
  isCollapsed: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Button
      variant="ghost"
      size="icon"
      // The ghost variant paints `aria-expanded:bg-muted`, which would give a
      // permanent grey while expanded — this button uses aria-expanded as a
      // disclosure state, not a select/trigger visual. Keep it transparent at
      // rest and grey only on hover, matching the update trigger beside it.
      className="h-11 w-11 cursor-pointer active:scale-95 sm:h-7 sm:w-7 aria-expanded:bg-transparent hover:bg-muted!"
      onClick={onToggle}
      data-testid={`collapse-agent-${agentName}`}
      aria-expanded={!isCollapsed}
      aria-label={
        isCollapsed
          ? t("agents:expandAgentProfiles", { name: displayName })
          : t("agents:collapseAgentProfiles", { name: displayName })
      }
    >
      <IconChevronDown
        className={`size-4 transition-transform ${isCollapsed ? "" : "rotate-180"}`}
      />
    </Button>
  );
}

/**
 * The profile count shown in the header while the block is collapsed — the
 * same copy the profile list's first line normally shows, so a collapsed block
 * never hides how many profiles the agent has (or that it has none yet). Renders
 * left of the update and collapse buttons.
 */
function CollapsedCountLabel({ agentName, count }: { agentName: string; count: number }) {
  const { t } = useTranslation();
  const label = count === 0 ? t("agents:noProfilesYetShort") : t("agents:profileCount", { count });
  return (
    <span
      className="min-w-0 text-sm text-muted-foreground"
      data-testid={`collapsed-count-${agentName}`}
    >
      {label}
    </span>
  );
}

/**
 * The header's primary action: "New profile" once the agent has a saved
 * record with profiles, "Setup Profile" otherwise (create mode when a saved
 * record exists so the first profile is drafted).
 */
function AgentProfileActionButton({
  agentName,
  configured,
  hasAgentRecord,
  agentHref,
}: {
  agentName: string;
  configured: boolean;
  hasAgentRecord: boolean;
  agentHref: string;
}) {
  const { t } = useTranslation();
  if (configured) {
    return (
      <Button className="h-11 cursor-pointer md:h-7" asChild>
        <Link href={`${agentHref}?mode=create`} data-testid={`new-profile-${agentName}`}>
          <IconPlus className="mr-2 h-4 w-4" />
          {t("agents:newProfile")}
        </Link>
      </Button>
    );
  }
  // Keep the setup action for agents without a usable profile. A saved record
  // needs create mode so its first profile is drafted.
  return (
    <Button className="h-11 cursor-pointer md:h-7" asChild>
      <Link
        href={hasAgentRecord ? `${agentHref}?mode=create` : agentHref}
        data-testid={`setup-profile-${agentName}`}
      >
        <IconSettings className="mr-2 h-4 w-4" />
        {t("agents:setupProfile")}
      </Link>
    </Button>
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
  runtimeUpdateStatus,
  updateJob,
  installJob,
  onPreview,
  onUpdate,
  onAuthComplete,
  children,
}: Props) {
  const { collapsed, setCollapsed } = useCollapsedAgentBlocks();
  const isCollapsed = collapsed(agent.name);
  const configured = Boolean(savedAgent && savedAgent.profiles.length > 0);
  const hasAgentRecord = Boolean(savedAgent);
  const agentHref = `/settings/agents/${encodeURIComponent(agent.name)}`;
  const [loginOpen, setLoginOpen] = useState(false);
  const [shellOpen, setShellOpen] = useState(false);
  const authRequired = capabilityStatus === "auth_required";
  const probing = capabilityStatus === "probing";
  const loginAvailable = Boolean(agent.login_command);
  const profileCount = savedAgent?.profiles.length ?? 0;

  // Either we have a registered login command (open the dedicated login PTY)
  // or we don't (open a plain host shell so the user can explore via
  // `<agent> --help`, run their own auth recipe, etc.).
  const handleAuthClick = () => {
    if (loginAvailable) setLoginOpen(true);
    else setShellOpen(true);
  };

  // The hook's `setCollapsed` throws when the write fails (quota / private
  // mode), matching the shared localStorage-preference contract. At this UI
  // boundary the failure must stay invisible: the current expanded/collapsed
  // snapshot remains authoritative and nothing escapes the click handler.
  const handleToggleCollapsed = () => {
    try {
      setCollapsed(agent.name, !isCollapsed);
    } catch {
      // No fallback worth surfacing: the preference just does not persist.
    }
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
        <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
          {isCollapsed && <CollapsedCountLabel agentName={agent.name} count={profileCount} />}
          {runtimeUpdate?.supported && onPreview && onUpdate && (
            <AgentRuntimeUpdateControl
              agentName={agent.name}
              displayName={displayName}
              runtimeUpdate={runtimeUpdate}
              runtimeUpdateStatus={runtimeUpdateStatus}
              job={updateJob}
              installJob={installJob}
              onPreview={onPreview}
              onUpdate={onUpdate}
            />
          )}
          <AgentCollapseControl
            agentName={agent.name}
            displayName={displayName}
            isCollapsed={isCollapsed}
            onToggle={handleToggleCollapsed}
          />
          <AgentProfileActionButton
            agentName={agent.name}
            configured={configured}
            hasAgentRecord={hasAgentRecord}
            agentHref={agentHref}
          />
        </div>
      </div>
      {/* Profiles area: full-bleed below the header, split off by a 1px border.
          Hidden while the card is collapsed. */}
      <Collapsible open={!isCollapsed}>
        <CollapsibleContent>{children}</CollapsibleContent>
      </Collapsible>
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
