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
import { useAppStore } from "@/components/state-provider";
import type {
  ActiveSessionInfo,
  AutomationReference,
  RoutingTierReference,
  WatcherReference,
  UtilityAgentReference,
} from "@/lib/types/agent-profile-errors";

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
  open,
  isFinePointer,
  anchorRef,
  onOpenChange,
  onCancel,
  onConfirm,
}: AgentProfileDeleteConfirmationProps) {
  const { t } = useTranslation();
  if (isFinePointer) {
    return (
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
    );
  }

  if (!open) return null;
  return (
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
}

export function AgentProfileDisableConflictDialog({
  agents,
  onCancel,
  onConfirm,
}: {
  agents: UtilityAgentReference[];
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog
      open={agents.length > 0}
      onOpenChange={(open) => {
        if (!open) onCancel();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("agents:disableProfile")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("agents:disableProfileUtilityWarning")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <ul className="list-disc list-inside max-h-40 overflow-y-auto">
          {agents.map((agent) => (
            <li key={agent.id}>{agent.name || agent.id}</li>
          ))}
        </ul>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t(CANCEL_LABEL_KEY)}</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm} className="cursor-pointer">
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
  const hasHardBlockers = routingTiers.length > 0;
  const watchersByKind = groupWatchersByKind(watchers);
  const workspaces = useAppStore((s) => s.workspaces.items);
  const providers = useAppStore((s) => s.settingsAgents.items);

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
            />
            <SessionConflictSection
              title={t("agents:conflictQuickChatsTitle")}
              sessions={quickChats}
              fallback={t("agents:untitledQuickChat")}
            />
            <WatcherConflictSection watchersByKind={watchersByKind} />
            <RoutingTierConflictSection
              routingTiers={routingTiers}
              workspaceLabels={new Map(workspaces.map((w) => [w.id, w.name]))}
              providerLabels={new Map(providers.map((p) => [p.id, p.name]))}
            />
            <AutomationConflictSection
              automations={automations}
              workspaceLabels={new Map(workspaces.map((w) => [w.id, w.name]))}
            />
            <UtilityAgentConflictSection utilityAgents={utilityAgents} />
            {hasHardBlockers ? (
              <p>{t("agents:changeTierMappingsFirst")}</p>
            ) : (
              <p>{t("agents:deleteAnywayConsequences")}</p>
            )}
          </div>
        </AlertDialogDescription>
        <AlertDialogFooter data-testid="agent-profile-delete-conflict-footer">
          <AlertDialogCancel className="min-h-11 w-full cursor-pointer sm:min-h-9 sm:w-auto">
            {t(CANCEL_LABEL_KEY)}
          </AlertDialogCancel>
          {hasHardBlockers ? null : (
            <AlertDialogAction
              onClick={onConfirm}
              className="min-h-11 w-full cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90 sm:min-h-9 sm:w-auto"
            >
              {t("agents:deleteAnyway")}
            </AlertDialogAction>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
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
}: {
  title: string;
  sessions: ActiveSessionInfo[];
  fallback: string;
}) {
  if (sessions.length === 0) return null;
  return (
    <div className="space-y-1">
      <p className="font-medium text-sm">{title}</p>
      <ul className="list-disc list-inside space-y-0.5">
        {sessions.map((t) => (
          <li key={t.task_id} className="text-sm">
            {t.task_title || fallback}
          </li>
        ))}
      </ul>
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
