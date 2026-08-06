"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconDotsVertical, IconPlus, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { AgentProfileDeleteConfirmDialog } from "@/components/settings/agent-profile-delete-dialog";
import { deleteAgentProfileAction } from "@/app/actions/agents";
import { useRouter } from "@/lib/routing/client-router";
import type { Agent, AgentProfile } from "@/lib/types/http";

function agentSetupHref(agentName: string): string {
  return `/settings/agents/${encodeURIComponent(agentName)}?mode=create`;
}

function profileHref(agentName: string, profileId: string): string {
  return `/settings/agents/${encodeURIComponent(agentName)}/profiles/${encodeURIComponent(profileId)}`;
}

/**
 * An agent's profiles inside its group card on the Agents page: a contrasted
 * body with a count, a prominent "New profile" action, and one clickable row
 * per profile (no agent branding — the group header already names the agent).
 */
export function AgentProfilesSubList({
  savedAgent,
  agentName,
}: {
  savedAgent: Agent | undefined;
  agentName: string;
}) {
  const { t } = useTranslation();
  const profiles = savedAgent?.profiles ?? [];
  return (
    <div
      className="border-t border-border/70 bg-background p-3 space-y-2"
      data-testid={`agent-profiles-${agentName}`}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium text-muted-foreground">
          {profiles.length === 0
            ? t("agents:noProfilesYet")
            : t("agents:profileCount", { count: profiles.length })}
        </span>
        <Button size="sm" className="cursor-pointer" asChild>
          <Link href={agentSetupHref(agentName)} data-testid={`new-profile-${agentName}`}>
            <IconPlus className="h-4 w-4 mr-2" />
            {t("agents:newProfile")}
          </Link>
        </Button>
      </div>
      {profiles.length > 0 && (
        <div className="grid gap-2">
          {profiles.map((profile) => (
            <ProfileRow key={profile.id} agent={savedAgent as Agent} profile={profile} />
          ))}
        </div>
      )}
    </div>
  );
}

/** One saved profile as a fully clickable row — shared by the Agents index and the agent page. */
export function ProfileRow({ agent, profile }: { agent: Agent; profile: AgentProfile }) {
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
    <Card
      // Same surface treatment as the workspace section tiles.
      className="relative gap-0 border-border/70 bg-background/50 py-1.5 transition-colors hover:border-foreground/30 hover:bg-muted/50"
      data-testid="agent-profile-row"
    >
      {/* Whole-card link as an overlay — the action buttons sit above it (z-10). */}
      <Link
        href={href}
        aria-label={profile.name}
        className="absolute inset-0"
        data-testid="agent-profile-row-link"
      />
      <CardContent className="flex items-center justify-between gap-2 px-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span aria-hidden className="h-1.5 w-1.5 shrink-0 rounded-full bg-primary/70" />
            <span className="truncate text-sm font-medium">{profile.name}</span>
          </div>
          {(profile.model || profile.mode) && (
            <div className="mt-0.5 flex flex-wrap items-center gap-1.5 pl-3.5">
              {profile.model && <Badge variant="outline">{profile.model}</Badge>}
              {profile.mode && <Badge variant="secondary">{profile.mode}</Badge>}
            </div>
          )}
        </div>
        <div className="relative z-10 flex shrink-0 items-center gap-1">
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
