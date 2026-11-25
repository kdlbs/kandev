"use client";

import { Trans, useTranslation } from "react-i18next";
import type { RefObject } from "react";
import type { TFunction } from "i18next";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { useAppStore } from "@/components/state-provider";
import type {
  ActiveSessionInfo,
  AutomationReference,
  DynamicProfileReference,
  RoutingTierReference,
  WatcherReference,
  UtilityAgentReference,
} from "@/lib/types/agent-profile-errors";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { groupSessionsByWorkspace } from "@/lib/settings/agent-profile-delete-conflict";

// The watcher `kind` values are the wire enum and are never translated; only
// their labels are copy, so they travel as catalog keys and resolve at render.
const WATCHER_KIND_LABEL_KEYS: Record<WatcherReference["kind"], string> = {
  linear: "agents:watcherKindLinear",
  jira: "agents:watcherKindJira",
  github_issue: "agents:watcherKindGithubIssue",
  github_review: "agents:watcherKindGithubReview",
};

const DELETE_PROFILE_TITLE_KEY = "agents:deleteAgentProfileTitle";
const DELETE_PROFILE_DESCRIPTION_KEY = "agents:deleteAgentProfileDescription";
const CANCEL_LABEL_KEY = "common:cancel";

type AgentProfileDeleteConfirmationProps = {
  profileId: string;
  profileName: string;
  open: boolean;
  isFinePointer: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
};

/**
 * Local confirmation for a simple profile deletion. Conflict handling stays in
 * AgentProfileDeleteConflictDialog because its dependency lists need a modal.
 */
export function AgentProfileDeleteConfirmation({
  profileId,
  profileName,
  open,
  isFinePointer,
  anchorRef,
  onOpenChange,
  onCancel,
  onConfirm,
}: AgentProfileDeleteConfirmationProps) {
  const { t } = useTranslation();
  const fallback = isFinePointer ? (
    <ActionConfirmPopover
      open={open}
      anchorRef={anchorRef}
      title={t(DELETE_PROFILE_TITLE_KEY)}
      description={t(DELETE_PROFILE_DESCRIPTION_KEY)}
      cancelLabel={t(CANCEL_LABEL_KEY)}
      confirmLabel={t("agents:delete")}
      confirmTestId="agent-profile-delete-confirm"
      testId="agent-profile-delete-confirm-popover"
      onOpenChange={onOpenChange}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  ) : (
    <InlineConfirmActions
      density="touch"
      testId="agent-profile-delete-inline-confirmation"
      ariaLabel={t(DELETE_PROFILE_TITLE_KEY)}
      description={t(DELETE_PROFILE_DESCRIPTION_KEY)}
      cancelLabel={t(CANCEL_LABEL_KEY)}
      confirmLabel={t("agents:delete")}
      confirmTestId="agent-profile-delete-confirm"
      onCancel={onCancel}
      onClose={() => onOpenChange(false)}
      onConfirm={onConfirm}
    />
  );
  return (
    <MobileActionConfirmation
      open={open}
      targetKey={profileId}
      title={t(DELETE_PROFILE_TITLE_KEY)}
      subject={profileName}
      description={t(DELETE_PROFILE_DESCRIPTION_KEY)}
      cancelLabel={t(CANCEL_LABEL_KEY)}
      confirmLabel={t("agents:delete")}
      confirmTestId="agent-profile-delete-confirm"
      focusReturnRef={anchorRef}
      onOpenChange={onOpenChange}
      onCancel={onCancel}
      onConfirm={onConfirm}
      fallback={fallback}
    />
  );
}

export function AgentProfileDisableConflictDialog({
  agents,
  dynamicProfiles = [],
  onCancel,
  onConfirm,
}: {
  agents: UtilityAgentReference[];
  dynamicProfiles?: DynamicProfileReference[];
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  const hasConflict = agents.length > 0 || dynamicProfiles.length > 0;
  const actionClassName = settingsActionClassName(
    "w-full cursor-pointer sm:w-auto max-md:min-h-12 " +
      "[@media(pointer:coarse)]:min-h-12 [@media(any-pointer:coarse)]:min-h-12",
  );
  return (
    <AlertDialog
      open={hasConflict}
      onOpenChange={(open) => {
        if (!open) onCancel();
      }}
    >
      <AlertDialogContent
        data-testid="agent-profile-disable-conflict-dialog"
        data-layout="contained"
        className="max-h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      >
        <AlertDialogHeader>
          <AlertDialogTitle>{t("agents:disableProfile")}</AlertDialogTitle>
        </AlertDialogHeader>
        <AlertDialogDescription
          asChild
          data-testid="disable-profile-conflict-body"
          className="min-h-0 min-w-0 space-y-2 overflow-x-hidden overflow-y-auto overscroll-contain text-left"
        >
          <div>
            <p>
              {[
                agents.length > 0 ? t("agents:disableProfileUtilityWarning") : "",
                dynamicProfiles.length > 0 ? t("agents:disableProfileDynamicWarning") : "",
              ]
                .filter(Boolean)
                .join(" ")}
            </p>
            {agents.length > 0 ? (
              <ul className="list-disc list-inside" data-testid="disable-conflict-utility-agents">
                {agents.map((agent) => (
                  <li key={agent.id}>{agent.name || agent.id}</li>
                ))}
              </ul>
            ) : null}
            {dynamicProfiles.length > 0 ? (
              <div className="space-y-1" data-testid="disable-conflict-dynamic-profiles">
                <p className="font-medium text-sm">{t("agents:conflictDynamicProfilesTitle")}</p>
                <ul className="list-disc list-inside">
                  {dynamicProfiles.map((profile) => (
                    <li key={profile.id}>{profile.name || profile.id}</li>
                  ))}
                </ul>
              </div>
            ) : null}
          </div>
        </AlertDialogDescription>
        <AlertDialogFooter data-testid="disable-profile-conflict-footer">
          <AlertDialogCancel className={actionClassName}>{t(CANCEL_LABEL_KEY)}</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm} className={actionClassName}>
            {t("agents:disableProfileAnyway")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// AgentProfileDeleteConflict carries the structured 409 payload from the
// backend. `open` is separate from the lists so a watcher-only conflict
// (no active sessions) still pops the dialog.
export type AgentProfileDeleteConflict = {
  activeSessions: ActiveSessionInfo[];
  watchers: WatcherReference[];
  routingTiers: RoutingTierReference[];
  automations: AutomationReference[];
  utilityAgents?: UtilityAgentReference[];
  dynamicProfiles?: DynamicProfileReference[];
};

type AgentProfileDeleteConflictDialogProps = {
  conflict: AgentProfileDeleteConflict | null;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

// eslint-disable-next-line complexity
export function AgentProfileDeleteConflictDialog({
  conflict,
  onOpenChange,
  onConfirm,
}: AgentProfileDeleteConflictDialogProps) {
  const { t } = useTranslation();
  const tasks = conflict?.activeSessions.filter((s) => !s.is_ephemeral) ?? [];
  const quickChats = conflict?.activeSessions.filter((s) => s.is_ephemeral) ?? [];
  const watchers = conflict?.watchers ?? [];
  const routingTiers = conflict?.routingTiers ?? [];
  const automations = conflict?.automations ?? [];
  const utilityAgents = conflict?.utilityAgents ?? [];
  const dynamicProfiles = conflict?.dynamicProfiles ?? [];
  const hasHardBlockers = routingTiers.length > 0;
  const watchersByKind = groupWatchersByKind(watchers);
  const workspaces = useAppStore((s) => s.workspaces.items);
  const providers = useAppStore((s) => s.settingsAgents.items);
  const workspaceLabels = new Map(workspaces.map((w) => [w.id, w.name]));

  return (
    <AlertDialog open={!!conflict} onOpenChange={onOpenChange}>
      <AlertDialogContent
        data-testid="agent-profile-delete-conflict-dialog"
        data-layout="contained"
        className="max-h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      >
        <AlertDialogHeader>
          <AlertDialogTitle>
            {hasHardBlockers ? t("agents:cannotDeleteAgentProfile") : t(DELETE_PROFILE_TITLE_KEY)}
          </AlertDialogTitle>
        </AlertDialogHeader>
        <AlertDialogDescription
          asChild
          data-testid="agent-profile-delete-conflict-body"
          className="min-h-0 min-w-0 space-y-2 overflow-x-hidden overflow-y-auto overscroll-contain text-left"
        >
          <div>
            <p>{t("agents:profileInUseIntro")}</p>
            <SessionConflictSection
              title={t("agents:conflictTasksTitle")}
              sessions={tasks}
              fallback={t("agents:untitledTask")}
              workspaceLabels={workspaceLabels}
            />
            <SessionConflictSection
              title={t("agents:conflictQuickChatsTitle")}
              sessions={quickChats}
              fallback={t("agents:untitledQuickChat")}
              workspaceLabels={workspaceLabels}
            />
            <WatcherConflictSection watchersByKind={watchersByKind} />
            <RoutingTierConflictSection
              routingTiers={routingTiers}
              workspaceLabels={workspaceLabels}
              providerLabels={new Map(providers.map((p) => [p.id, p.name]))}
            />
            <AutomationConflictSection
              automations={automations}
              workspaceLabels={workspaceLabels}
            />
            <DynamicProfileConflictSection dynamicProfiles={dynamicProfiles} />
            <UtilityAgentConflictSection utilityAgents={utilityAgents} />
            {hasHardBlockers ? (
              <p>{t("agents:changeTierMappingsFirst")}</p>
            ) : (
              <p>{t("agents:deleteAnywayConsequences")}</p>
            )}
          </div>
        </AlertDialogDescription>
        <AlertDialogFooter data-testid="agent-profile-delete-conflict-footer">
          <AlertDialogCancel className={settingsActionClassName("w-full cursor-pointer sm:w-auto")}>
            {t(CANCEL_LABEL_KEY)}
          </AlertDialogCancel>
          {hasHardBlockers ? null : (
            <AlertDialogAction
              onClick={onConfirm}
              className={settingsActionClassName(
                "w-full cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90 sm:w-auto",
              )}
            >
              {t("agents:deleteAnyway")}
            </AlertDialogAction>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// Dynamic profiles route through this profile as one of their candidates.
// Deleting it leaves those routes pointing at a profile that no longer
// exists, so the user must see them before choosing "Delete Anyway".
function DynamicProfileConflictSection({
  dynamicProfiles,
}: {
  dynamicProfiles: DynamicProfileReference[];
}) {
  const { t } = useTranslation();
  if (dynamicProfiles.length === 0) return null;
  return (
    <div className="space-y-1" data-testid="profile-conflict-dynamic-profiles">
      <p className="font-medium text-sm">{t("agents:conflictDynamicProfilesTitle")}</p>
      <ul className="list-disc list-inside space-y-0.5">
        {dynamicProfiles.map((profile) => (
          <li key={profile.id} className="text-sm">
            {profile.name || profile.id}
          </li>
        ))}
      </ul>
    </div>
  );
}

function UtilityAgentConflictSection({
  utilityAgents,
}: {
  utilityAgents: UtilityAgentReference[];
}) {
  const { t } = useTranslation();
  if (utilityAgents.length === 0) return null;
  return (
    <div className="space-y-1" data-testid="profile-conflict-utility-agents">
      <p className="font-medium text-sm">{t("agents:conflictUtilityAgentsTitle")}</p>
      <ul className="list-disc list-inside space-y-0.5">
        {utilityAgents.map((agent) => (
          <li key={agent.id} className="text-sm">
            {agent.name || agent.id}
          </li>
        ))}
      </ul>
    </div>
  );
}

function SessionConflictSection({
  title,
  sessions,
  fallback,
  workspaceLabels,
}: {
  title: string;
  sessions: ActiveSessionInfo[];
  fallback: string;
  workspaceLabels: Map<string, string>;
}) {
  const { t } = useTranslation();
  if (sessions.length === 0) return null;
  return (
    <div className="space-y-1">
      <p className="font-medium text-sm">{title}</p>
      {groupSessionsByWorkspace(sessions, (id) => workspaceLabels.get(id)).map((group) => (
        <div
          key={group.workspaceId}
          className="space-y-0.5 pl-2"
          data-testid="profile-conflict-workspace-group"
        >
          <h3
            className="text-sm font-medium break-words"
            data-testid="profile-conflict-workspace-name"
          >
            {group.workspaceName ||
              workspaceLabels.get(group.workspaceId) ||
              t("agents:conflictUnknownWorkspace")}
          </h3>
          <ul className="list-disc list-inside space-y-0.5 pl-2">
            {group.sessions.map((s) => (
              <li key={s.task_id} className="text-sm">
                {s.task_title || fallback}
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
}

function WatcherConflictSection({
  watchersByKind,
}: {
  watchersByKind: Record<string, WatcherReference[]>;
}) {
  const { t } = useTranslation();
  const entries = Object.entries(watchersByKind);
  if (entries.length === 0) return null;
  return (
    <div className="space-y-1">
      <p className="font-medium text-sm">{t("agents:conflictWatchersTitle")}</p>
      <ul className="list-disc list-inside space-y-0.5">
        {entries.map(([kind, items]) => (
          <li key={kind} className="text-sm">
            <span className="font-medium">
              {watcherKindLabel(t, kind as WatcherReference["kind"])}:
            </span>{" "}
            {items.map((w) => w.label || w.id).join(", ")}
          </li>
        ))}
      </ul>
    </div>
  );
}

// Enabled automations bound to this profile. Listed separately from sessions
// because nothing is running: an automation is a standing instruction, and the
// damage shows up on its next firing rather than now.
function AutomationConflictSection({
  automations,
  workspaceLabels,
}: {
  automations: AutomationReference[];
  workspaceLabels: Map<string, string>;
}) {
  const { t } = useTranslation();
  if (automations.length === 0) return null;
  return (
    <div className="space-y-1" data-testid="delete-conflict-automations">
      <p className="font-medium text-sm">{t("agents:conflictAutomationsTitle")}</p>
      <ul className="list-disc list-inside space-y-0.5">
        {automations.map((ref) => (
          <li key={ref.id} className="text-sm">
            <Trans
              i18nKey="agents:conflictAutomationRow"
              values={{
                name: ref.name,
                workspace: formatLookupLabel(workspaceLabels, ref.workspace_id),
              }}
            >
              <span className="font-medium" />
            </Trans>
          </li>
        ))}
      </ul>
    </div>
  );
}

function RoutingTierConflictSection({
  routingTiers,
  workspaceLabels,
  providerLabels,
}: {
  routingTiers: RoutingTierReference[];
  workspaceLabels: Map<string, string>;
  providerLabels: Map<string, string>;
}) {
  const { t } = useTranslation();
  if (routingTiers.length === 0) return null;
  return (
    <div className="space-y-1">
      <p className="font-medium text-sm">{t("agents:conflictTierMappingsTitle")}</p>
      <ul className="list-disc list-inside space-y-0.5">
        {routingTiers.map((ref) => (
          <li key={`${ref.workspace_id}-${ref.provider_id}-${ref.tier}`} className="text-sm">
            <Trans
              i18nKey="agents:conflictTierMappingRow"
              values={{
                tier: formatRoutingTier(ref.tier),
                workspace: formatLookupLabel(workspaceLabels, ref.workspace_id),
                provider: formatLookupLabel(providerLabels, ref.provider_id),
              }}
            >
              <span className="font-medium" />
            </Trans>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * The watcher `kind` is the wire enum; only its label is copy. An unknown kind
 * echoes the raw value rather than inventing one.
 */
function watcherKindLabel(t: TFunction, kind: WatcherReference["kind"]): string {
  const key = WATCHER_KIND_LABEL_KEYS[kind];
  return key ? t(key) : kind;
}

/**
 * `tier` is a routing wire value (`fast`, `heavy`, …), not copy — it is
 * title-cased for display and never translated.
 */
function formatRoutingTier(tier: string): string {
  return tier.charAt(0).toUpperCase() + tier.slice(1);
}

function formatLookupLabel(labels: Map<string, string>, id: string): string {
  const label = labels.get(id);
  return label && label !== id ? `${label} (${id})` : id;
}

function groupWatchersByKind(watchers: WatcherReference[]): Record<string, WatcherReference[]> {
  return watchers.reduce<Record<string, WatcherReference[]>>((acc, w) => {
    (acc[w.kind] ??= []).push(w);
    return acc;
  }, {});
}
