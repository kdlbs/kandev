"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconDotsVertical, IconPencil, IconPlus, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { AgentLogo } from "@/components/agent-logo";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { AgentProfileDeleteConfirmDialog } from "@/components/settings/agent-profile-delete-dialog";
import { deleteAgentProfileAction } from "@/app/actions/agents";
import { useRouter } from "@/lib/routing/client-router";
import { AGENTS_BROWSE_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/agents";
import type { Agent, AgentProfile } from "@/lib/types/http";

function agentSetupHref(agentName: string): string {
  return `/settings/agents/${encodeURIComponent(agentName)}?mode=create`;
}

function profileHref(agentName: string, profileId: string): string {
  return `/settings/agents/${encodeURIComponent(agentName)}/profiles/${encodeURIComponent(profileId)}`;
}

function NewProfileButton({ agents }: { agents: Agent[] }) {
  const { t } = useTranslation();
  const router = useRouter();

  if (agents.length === 0) {
    return (
      <Button size="sm" className="cursor-pointer" asChild>
        <Link href={AGENTS_BROWSE_SETTINGS_HREF}>
          <IconPlus className="h-4 w-4 mr-2" />
          {t("agents:newProfile")}
        </Link>
      </Button>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" className="cursor-pointer" data-testid="new-agent-profile">
          <IconPlus className="h-4 w-4 mr-2" />
          {t("agents:newProfile")}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {agents.map((agent) => (
          <DropdownMenuItem
            key={agent.name}
            className="cursor-pointer"
            onSelect={() => router.push(agentSetupHref(agent.name))}
          >
            <AgentLogo agentName={agent.name} className="h-3.5 w-3.5 mr-2 shrink-0" />
            {agent.profiles[0]?.agentDisplayName ?? agent.name}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ProfileRow({ agent, profile }: { agent: Agent; profile: AgentProfile }) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const router = useRouter();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const href = profileHref(agent.name, profile.id);

  const handleDelete = async () => {
    setConfirmOpen(false);
    const result = await deleteAgentProfileAction(profile.id);
    if (result.status === "ok") {
      setSettingsAgents(
        settingsAgents.map((item) => ({
          ...item,
          profiles: item.profiles.filter((p) => p.id !== profile.id),
        })),
      );
      return;
    }
    // Conflicts (active sessions, watchers, routing tiers) carry a guided
    // resolution flow that lives on the profile page — send the user there.
    if (result.status === "conflict") {
      toast({ title: t("agents:cannotDeleteAgentProfile"), variant: "error" });
      router.push(href);
      return;
    }
    toast({ title: t("agents:cannotDeleteAgentProfile"), description: result.message, variant: "error" });
  };

  return (
    <Card className="relative transition-colors hover:bg-muted/50" data-testid="agent-profile-row">
      {/* Whole-card link as an overlay — the action buttons sit above it (z-10). */}
      <Link
        href={href}
        aria-label={profile.name || profile.agentDisplayName || agent.name}
        className="absolute inset-0"
        data-testid="agent-profile-row-link"
      />
      <CardContent className="py-2 flex items-center justify-between gap-2">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <AgentLogo agentName={agent.name} className="shrink-0" />
          <span className="text-sm font-medium truncate">
            {profile.agentDisplayName || agent.name}
          </span>
          <span className="text-sm text-muted-foreground truncate">{profile.name}</span>
          {profile.model && (
            <Badge variant="outline" className="hidden sm:inline-flex">
              {profile.model}
            </Badge>
          )}
          {profile.mode && (
            <Badge variant="secondary" className="hidden sm:inline-flex">
              {profile.mode}
            </Badge>
          )}
        </div>
        <div className="relative z-10 flex shrink-0 items-center gap-1">
          <Button variant="ghost" size="sm" className="cursor-pointer" asChild>
            <Link href={href} aria-label={t("common:edit")}>
              <IconPencil className="h-4 w-4" />
            </Link>
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="cursor-pointer"
                aria-label={t("agents:profileActions")}
              >
                <IconDotsVertical className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                className="cursor-pointer text-destructive focus:text-destructive"
                onSelect={() => setConfirmOpen(true)}
              >
                <IconTrash className="h-4 w-4 mr-2" />
                {t("agents:delete")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </CardContent>
      <AgentProfileDeleteConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        onConfirm={() => void handleDelete()}
      />
    </Card>
  );
}

/**
 * Every configured profile across all agents — the first thing on the Agents
 * page. Profiles are user data: they live here as a list, never as menu rows.
 */
export function AgentProfilesSection({ savedAgents }: { savedAgents: Agent[] }) {
  const { t } = useTranslation();
  const configuredAgents = savedAgents.filter((agent) => agent.profiles.length > 0);

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h3 className="text-lg font-semibold">{t("agents:agentProfiles")}</h3>
          <p className="text-sm text-muted-foreground">{t("agents:agentProfilesDescription")}</p>
        </div>
        <NewProfileButton agents={savedAgents} />
      </div>

      {configuredAgents.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center">
            <p className="text-sm text-muted-foreground">{t("agents:noProfilesYet")}</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-2">
          {configuredAgents.flatMap((agent) =>
            agent.profiles.map((profile) => (
              <ProfileRow key={profile.id} agent={agent} profile={profile} />
            )),
          )}
        </div>
      )}
    </div>
  );
}
