"use client";

import { useRef, useState, type RefObject, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconCopy, IconDotsVertical, IconGripVertical, IconTrash } from "@tabler/icons-react";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  pointerWithin,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { profileDragAccessibility } from "@/lib/settings/profile-drag-accessibility";
import { reorderIds } from "@/lib/settings/agent-profile-order";
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
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useConfirmationBoundary } from "@/components/confirmation/mobile-action-confirmation";
import { useRouter } from "@/lib/routing/client-router";
import { classifyAgentProfileFallback } from "@/lib/agent-profile-fallback";
import type { Agent, AgentProfile } from "@/lib/types/http";
import { RecordDot } from "@/components/settings/record-dot";
import { DisabledBadge } from "@/components/settings/record-badges";
import { settingsActionClassName } from "@/components/settings/settings-control";

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
  canManage,
  onReorder,
}: {
  savedAgent: Agent | undefined;
  agentName: string;
  canManage: boolean;
  onReorder: (agentId: string, profileIds: string[]) => void;
}) {
  const { t } = useTranslation();
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  if (!savedAgent || savedAgent.profiles.length === 0) return null;
  const rows = savedAgent.profiles.map((profile) => (
    <ProfileRow key={profile.id} agent={savedAgent} profile={profile} />
  ));
  return (
    <div
      className="border-t border-border/70 bg-background p-3"
      data-testid={`agent-profiles-${agentName}`}
    >
      {canManage && savedAgent.profiles.length > 1 ? (
        <DndContext
          accessibility={profileDragAccessibility(savedAgent.profiles, t)}
          sensors={sensors}
          collisionDetection={(args) =>
            args.pointerCoordinates ? pointerWithin(args) : closestCenter(args)
          }
          onDragEnd={(event: DragEndEvent) => {
            if (!event.over || event.active.id === event.over.id) return;
            onReorder(
              savedAgent.id,
              reorderIds(
                savedAgent.profiles.map((profile) => profile.id),
                String(event.active.id),
                String(event.over.id),
              ),
            );
          }}
        >
          <SortableContext
            items={savedAgent.profiles.map((profile) => profile.id)}
            strategy={verticalListSortingStrategy}
          >
            <div className="grid gap-2">
              {savedAgent.profiles.map((profile) => (
                <SortableProfileRow key={profile.id} agent={savedAgent} profile={profile} />
              ))}
            </div>
          </SortableContext>
        </DndContext>
      ) : (
        <div className="grid gap-2">{rows}</div>
      )}
    </div>
  );
}
function SortableProfileRow({ agent, profile }: { agent: Agent; profile: AgentProfile }) {
  const { t } = useTranslation();
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: profile.id });
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={isDragging ? "z-20 opacity-70" : undefined}
    >
      <ProfileRow
        agent={agent}
        profile={profile}
        dragHandle={
          <button
            ref={setActivatorNodeRef}
            type="button"
            className={controlSizingClassName(
              "icon",
              "relative z-10 flex shrink-0 cursor-pointer touch-none items-center justify-center rounded-md text-muted-foreground hover:bg-muted",
            )}
            data-testid="agent-profile-drag-handle"
            {...attributes}
            {...listeners}
            aria-label={t("agents:dragProfile", { name: profile.name })}
            aria-roledescription={t("agents:profileSortable")}
          >
            <IconGripVertical className="h-4 w-4" aria-hidden="true" />
          </button>
        }
      />
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
  duplicateDisabled,
}: {
  profile: AgentProfile;
  deleteAnchorRef: RefObject<HTMLButtonElement | null>;
  onDuplicate: () => void;
  onConfirmDelete: () => void;
  duplicateDisabled: boolean;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const pendingDelete = useRef(false);
  const [open, setOpen] = useState(false);
  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger
        asChild
        onPointerDown={(event) => {
          event.preventDefault();
          event.stopPropagation();
        }}
        onClick={(event) => {
          event.stopPropagation();
          setOpen((current) => !current);
        }}
      >
        <Button
          ref={deleteAnchorRef}
          variant="ghost"
          size="icon"
          className={settingsActionClassName("cursor-pointer")}
          aria-label={t("agents:profileActions")}
          data-testid={`profile-actions-menu-${profile.id}`}
        >
          <IconDotsVertical className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        onCloseAutoFocus={(event) => {
          if (!pendingDelete.current) return;
          pendingDelete.current = false;
          event.preventDefault();
          onConfirmDelete();
        }}
      >
        {profile.kind !== "dynamic" && (
          <DropdownMenuItem
            className="cursor-pointer"
            data-testid={`duplicate-profile-${profile.id}`}
            disabled={duplicateDisabled}
            onSelect={onDuplicate}
          >
            <IconCopy className="h-4 w-4 mr-2" />
            {t("agents:duplicate")}
          </DropdownMenuItem>
        )}
        <DropdownMenuItem
          className="cursor-pointer text-destructive focus:text-destructive"
          data-testid={`delete-profile-${profile.id}`}
          onSelect={() => {
            if (isMobile) pendingDelete.current = true;
            else onConfirmDelete();
          }}
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
  duplicateDisabled,
}: {
  profile: AgentProfile;
  deleteAnchorRef: RefObject<HTMLButtonElement | null>;
  onDuplicate: () => void;
  onConfirmDelete: () => void;
  duplicateDisabled: boolean;
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
            disabled={duplicateDisabled}
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
  profileId: string;
  profileName: string;
  open: boolean;
  isFinePointer: boolean;
  anchorRef: RefObject<HTMLButtonElement | null>;
  onOpenChange: (open: boolean) => void;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
  placement: "inline" | "popover";
};
type ProfileRowDeleteConfirmationBaseProps = Omit<ProfileRowDeleteConfirmationProps, "placement">;

function ProfileRowDeleteConfirmation({
  profileId,
  profileName,
  open,
  isFinePointer,
  anchorRef,
  onOpenChange,
  onCancel,
  onConfirm,
  placement,
}: ProfileRowDeleteConfirmationProps) {
  const { isMobile } = useResponsiveBreakpoint();
  if (placement === "inline" && (isMobile || isFinePointer || !open)) return null;
  if (placement === "popover" && !isMobile && !isFinePointer) return null;

  const confirmation = (
    <AgentProfileDeleteConfirmation
      profileId={profileId}
      profileName={profileName}
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

type ProfileRowCardProps = {
  profile: AgentProfile;
  href: string;
  canManage: boolean;
  confirmOpen: boolean;
  isFinePointer: boolean;
  isFullDesktop: boolean;
  deleteAnchorRef: RefObject<HTMLButtonElement | null>;
  onDuplicate: () => void;
  onConfirmDelete: () => void;
  confirmationProps: ProfileRowDeleteConfirmationBaseProps;
  duplicateDisabled: boolean;
  dragHandle?: ReactNode;
};

function ProfileRowCard({
  profile,
  href,
  canManage,
  confirmOpen,
  isFinePointer,
  isFullDesktop,
  deleteAnchorRef,
  onDuplicate,
  onConfirmDelete,
  confirmationProps,
  duplicateDisabled,
  dragHandle,
}: ProfileRowCardProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  const fallbackState = classifyAgentProfileFallback(profile);
  let fallbackLabel = t("agents:fallbackNone");
  if (fallbackState.kind === "exact") {
    fallbackLabel = t("agents:fallbackExact");
  } else if (fallbackState.kind === "next") {
    fallbackLabel = t("agents:fallbackNext");
  } else if (fallbackState.kind === "model") {
    fallbackLabel = t("agents:fallbackModel", { model: fallbackState.model });
  }
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
          <div className="mt-0.5 flex flex-wrap items-center gap-1.5 pl-3.5">
            {profile.model && <Badge variant="outline">{profile.model}</Badge>}
            <Badge
              className="h-auto min-h-5 max-w-full min-w-0 overflow-visible whitespace-pre-wrap break-all text-left"
              variant="secondary"
            >
              {fallbackLabel}
            </Badge>
            {profile.mode && <Badge variant="secondary">{profile.mode}</Badge>}
          </div>
        </div>
        <div className="relative z-10 flex shrink-0 items-center gap-1">
          {dragHandle}
          {canManage &&
            (isMobile || !(confirmOpen && !isFinePointer)) &&
            (isFullDesktop ? (
              <ProfileRowInlineActions
                profile={profile}
                deleteAnchorRef={deleteAnchorRef}
                onDuplicate={onDuplicate}
                onConfirmDelete={onConfirmDelete}
                duplicateDisabled={duplicateDisabled}
              />
            ) : (
              <ProfileRowActions
                profile={profile}
                deleteAnchorRef={deleteAnchorRef}
                onDuplicate={onDuplicate}
                onConfirmDelete={onConfirmDelete}
                duplicateDisabled={duplicateDisabled}
              />
            ))}
        </div>
        <ProfileRowDeleteConfirmation {...confirmationProps} placement="inline" />
      </CardContent>
      <ProfileRowDeleteConfirmation {...confirmationProps} placement="popover" />
    </Card>
  );
}

/** One saved profile as a fully clickable row — shared by the Agents index and the agent page. */
export function ProfileRow({
  agent,
  profile,
  dragHandle,
}: {
  agent: Agent;
  profile: AgentProfile;
  dragHandle?: ReactNode;
}) {
  const canManage = useIsAdmin();
  const { t } = useTranslation();
  const { toast } = useToast();
  const router = useRouter();
  const { isFinePointer, isFullDesktop } = useResponsiveBreakpoint();
  const handleDuplicate = useProfileDuplicate();
  const [confirmOpen, setConfirmOpen] = useState(false);
  useConfirmationBoundary(confirmOpen, profile.id, setConfirmOpen);
  const deleteAnchorRef = useRef<HTMLButtonElement>(null);
  const store = useAppStoreApi();
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const nativeCodexAvailable = useAppStore((state) => state.features?.codexAppServer ?? false);
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
      // Deletion preserves selector order and profiles absent from the Settings list.
      setAgentProfiles(
        store.getState().agentProfiles.items.filter((item) => item.id !== profile.id),
      );
      store.getState().bumpAgentProfilesVersion();
      return;
    }
    // Conflicts (active sessions, watchers, routing tiers) carry a guided
    // resolution flow that lives on the profile page — send the user there.
    if (result.status === "conflict") {
      toast({ title: t("agents:cannotDeleteAgentProfile"), variant: "error" });
      router.push(href);
      return;
    }
    if (result.handled) {
      closeDeleteConfirmation();
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
    profileId: profile.id,
    profileName: profile.name,
    open: confirmOpen,
    isFinePointer,
    anchorRef: deleteAnchorRef,
    onOpenChange: setConfirmOpen,
    onCancel: closeDeleteConfirmation,
    onConfirm: () => void handleDelete(),
  };
  return (
    <ProfileRowCard
      profile={profile}
      href={href}
      canManage={canManage}
      confirmOpen={confirmOpen}
      isFinePointer={isFinePointer}
      isFullDesktop={isFullDesktop}
      deleteAnchorRef={deleteAnchorRef}
      onDuplicate={() => void handleDuplicate(agent, profile)}
      onConfirmDelete={() => setConfirmOpen(true)}
      confirmationProps={confirmationProps}
      duplicateDisabled={agent.name === "codex-app-server" && !nativeCodexAvailable}
      dragHandle={dragHandle}
    />
  );
}
