"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconCopy, IconDotsVertical, IconTrash } from "@tabler/icons-react";
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
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { AgentProfileDeleteConfirmDialog } from "@/components/settings/agent-profile-delete-dialog";
import { deleteAgentProfileAction } from "@/app/actions/agents";
import { useProfileDuplicate } from "@/hooks/domains/settings/use-profile-duplicate";
import { useRouter } from "@/lib/routing/client-router";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Agent, AgentProfile } from "@/lib/types/http";
import { RecordDot } from "@/components/settings/record-dot";
import { DisabledBadge } from "@/components/settings/record-badges";

function profileHref(agentName: string, profileId: string): string {
  return `/settings/agents/${encodeURIComponent(agentName)}/profiles/${encodeURIComponent(profileId)}`;
}

/**
 * An agent's profiles inside its group card on the Agents page: one clickable
 * row per profile (no agent branding or duplicate creation controls because
 * the group header already names the agent and owns its action).
 */
export function AgentProfilesSubList({
  savedAgent,
  agentName,
}: {
  savedAgent: Agent | undefined;
  agentName: string;
}) {
  if (!savedAgent || savedAgent.profiles.length === 0) return null;

  return (
    <div
      className="border-t border-border/70 bg-background p-3"
      data-testid={`agent-profiles-${agentName}`}
    >
      <div className="grid gap-2">
        {savedAgent.profiles.map((profile) => (
          <ProfileRow key={profile.id} agent={savedAgent} profile={profile} />
        ))}
      </div>
    </div>
  );
}

/**
 * The per-profile actions dropdown (duplicate, delete). The trigger exposes a
 * touch-sized hitbox (>= 44px) so the row actions are reachable on mobile.
 * Duplication uses the shared useProfileDuplicate hook (per-profile in-flight
 * guard, revision-aware store merge).
 */
function ProfileRowActions({
  agent,
  profile,
  onConfirmDelete,
}: {
  agent: Agent;
  profile: AgentProfile;
  onConfirmDelete: () => void;
}) {
  const { t } = useTranslation();
  const handleDuplicate = useProfileDuplicate();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="cursor-pointer min-h-11 min-w-11"
          aria-label={t("agents:profileActions")}
        >
          <IconDotsVertical className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {profile.kind !== "dynamic" && (
          <DropdownMenuItem
            className="cursor-pointer"
            data-testid={`duplicate-profile-${profile.id}`}
            onSelect={() => void handleDuplicate(agent, profile)}
          >
            <IconCopy className="h-4 w-4 mr-2" />
            {t("agents:duplicate")}
          </DropdownMenuItem>
        )}
        <DropdownMenuItem
          className="cursor-pointer text-destructive focus:text-destructive"
          onSelect={onConfirmDelete}
        >
          <IconTrash className="h-4 w-4 mr-2" />
          {t("agents:delete")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** One saved profile as a fully clickable row — shared by the Agents index and the agent page. */
export function ProfileRow({ agent, profile }: { agent: Agent; profile: AgentProfile }) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const router = useRouter();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const store = useAppStoreApi();
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const href = profileHref(agent.name, profile.id);

  const handleDelete = async () => {
    setConfirmOpen(false);
    const result = await deleteAgentProfileAction(profile.id);
    if (result.status === "ok") {
      // Read the store at write time, not at render: this closure was created
      // before the await, so a snapshot taken then would be stale by now and
      // two profiles deleted in quick succession would resurrect each other.
      const nextAgents = store.getState().settingsAgents.items.map((item) => ({
        ...item,
        profiles: item.profiles.filter((p) => p.id !== profile.id),
      }));
      setSettingsAgents(nextAgents);
      // `agentProfiles` is the flattened picker list over the same data. Every
      // other writer updates the pair together, and its only refetch is a
      // one-shot guarded by `agentsLoaded`, so skipping it here left the
      // deleted profile selectable until a reload.
      setAgentProfiles(
        nextAgents.flatMap((item) => item.profiles.map((p) => toAgentProfileOption(item, p))),
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
    toast({
      title: t("agents:cannotDeleteAgentProfile"),
      description: result.message,
      variant: "error",
    });
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
            <RecordDot />
            <span className="truncate text-sm font-medium">{profile.name}</span>
            {profile.enabled === false && <DisabledBadge />}
          </div>
          {(profile.model || profile.mode) && (
            <div className="mt-0.5 flex flex-wrap items-center gap-1.5 pl-3.5">
              {profile.model && <Badge variant="outline">{profile.model}</Badge>}
              {profile.mode && <Badge variant="secondary">{profile.mode}</Badge>}
            </div>
          )}
        </div>
        <div className="relative z-10 flex shrink-0 items-center gap-1">
          <ProfileRowActions
            agent={agent}
            profile={profile}
            onConfirmDelete={() => setConfirmOpen(true)}
          />
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
