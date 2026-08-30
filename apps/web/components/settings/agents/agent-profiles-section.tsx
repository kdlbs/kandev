"use client";

import { useRef, useState, type RefObject } from "react";
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
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import Link from "@/components/routing/app-link";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { AgentProfileDeleteConfirmation } from "@/components/settings/agent-profile-delete-dialog";
import { deleteAgentProfileAction } from "@/app/actions/agents";
import { useProfileDuplicate } from "@/hooks/domains/settings/use-profile-duplicate";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
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
  profile,
  deleteAnchorRef,
  onDuplicate,
  onConfirmDelete,
}: {
  profile: AgentProfile;
  deleteAnchorRef: RefObject<HTMLButtonElement | null>;
  onDuplicate: () => void;
  onConfirmDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          ref={deleteAnchorRef}
          variant="ghost"
          size="sm"
          className="cursor-pointer min-h-11 min-w-11"
          aria-label={t("agents:profileActions")}
          data-testid={`profile-actions-menu-${profile.id}`}
        >
          <IconDotsVertical className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {profile.kind !== "dynamic" && (
          <DropdownMenuItem
            className="cursor-pointer"
            data-testid={`duplicate-profile-${profile.id}`}
            onSelect={onDuplicate}
          >
            <IconCopy className="h-4 w-4 mr-2" />
            {t("agents:duplicate")}
          </DropdownMenuItem>
        )}
        <DropdownMenuItem
          className="cursor-pointer text-destructive focus:text-destructive"
          data-testid={`delete-profile-${profile.id}`}
          onSelect={onConfirmDelete}
        >
          <IconTrash className="h-4 w-4 mr-2" />
          {t("agents:delete")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ProfileRowInlineActions({
  profile,
  deleteAnchorRef,
  onDuplicate,
  onConfirmDelete,
}: {
  profile: AgentProfile;
  deleteAnchorRef: RefObject<HTMLButtonElement | null>;
  onDuplicate: () => void;
  onConfirmDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-1" data-testid={`profile-actions-inline-${profile.id}`}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="icon"
            className="cursor-pointer"
            data-testid={`duplicate-profile-inline-${profile.id}`}
            onClick={onDuplicate}
            aria-label={t("agents:duplicate")}
          >
            <IconCopy className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">{t("agents:duplicate")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            ref={deleteAnchorRef}
            type="button"
            variant="destructive"
            size="icon"
            className="cursor-pointer"
            data-testid={`delete-profile-inline-${profile.id}`}
            onClick={onConfirmDelete}
            aria-label={t("agents:delete")}
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">{t("agents:delete")}</TooltipContent>
      </Tooltip>
    </div>
  );
}

type ProfileRowDeleteConfirmationProps = {
  open: boolean;
  isFinePointer: boolean;
  anchorRef: RefObject<HTMLButtonElement | null>;
  onOpenChange: (open: boolean) => void;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
  placement: "inline" | "popover";
};

function ProfileRowDeleteConfirmation({
  open,
  isFinePointer,
  anchorRef,
  onOpenChange,
  onCancel,
  onConfirm,
  placement,
}: ProfileRowDeleteConfirmationProps) {
  if (placement === "inline" && (isFinePointer || !open)) return null;
  if (placement === "popover" && !isFinePointer) return null;

  const confirmation = (
    <AgentProfileDeleteConfirmation
      open={open}
      isFinePointer={isFinePointer}
      anchorRef={anchorRef}
      onOpenChange={onOpenChange}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );
  return placement === "inline" ? (
    <div className="relative z-10 basis-full min-w-0">{confirmation}</div>
  ) : (
    confirmation
  );
}

/** One saved profile as a fully clickable row — shared by the Agents index and the agent page. */
export function ProfileRow({ agent, profile }: { agent: Agent; profile: AgentProfile }) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const router = useRouter();
  const { isFinePointer, isFullDesktop } = useResponsiveBreakpoint();
  const handleDuplicate = useProfileDuplicate();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const deleteAnchorRef = useRef<HTMLButtonElement>(null);
  const store = useAppStoreApi();
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const href = profileHref(agent.name, profile.id);
  const closeDeleteConfirmation = () => {
    setConfirmOpen(false);
    queueMicrotask(() => deleteAnchorRef.current?.focus());
  };
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
    closeDeleteConfirmation();
  };
  const confirmationProps = {
    open: confirmOpen,
    isFinePointer,
    anchorRef: deleteAnchorRef,
    onOpenChange: setConfirmOpen,
    onCancel: closeDeleteConfirmation,
    onConfirm: () => void handleDelete(),
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
          {!(confirmOpen && !isFinePointer) &&
            (isFullDesktop ? (
              <ProfileRowInlineActions
                profile={profile}
                deleteAnchorRef={deleteAnchorRef}
                onDuplicate={() => void handleDuplicate(agent, profile)}
                onConfirmDelete={() => setConfirmOpen(true)}
              />
            ) : (
              <ProfileRowActions
                profile={profile}
                deleteAnchorRef={deleteAnchorRef}
                onDuplicate={() => void handleDuplicate(agent, profile)}
                onConfirmDelete={() => setConfirmOpen(true)}
              />
            ))}
        </div>
        <ProfileRowDeleteConfirmation {...confirmationProps} placement="inline" />
      </CardContent>
      <ProfileRowDeleteConfirmation {...confirmationProps} placement="popover" />
    </Card>
  );
}
