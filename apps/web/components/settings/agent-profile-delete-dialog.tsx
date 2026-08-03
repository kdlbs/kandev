"use client";

import { Trans, useTranslation } from "react-i18next";
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
import { useAppStore } from "@/components/state-provider";
import type {
  ActiveSessionInfo,
  RoutingTierReference,
  WatcherReference,
} from "@/lib/types/agent-profile-errors";

// The watcher `kind` values are the wire enum and are never translated; only
// their labels are copy, so they travel as catalog keys and resolve at render.
const WATCHER_KIND_LABEL_KEYS: Record<WatcherReference["kind"], string> = {
  linear: "agents:watcherKindLinear",
  jira: "agents:watcherKindJira",
  github_issue: "agents:watcherKindGithubIssue",
  github_review: "agents:watcherKindGithubReview",
};

type AgentProfileDeleteConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function AgentProfileDeleteConfirmDialog({
  open,
  onOpenChange,
  onConfirm,
}: AgentProfileDeleteConfirmDialogProps) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("agents:deleteAgentProfileTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("agents:deleteAgentProfileDescription")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {t("agents:delete")}
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
};

type AgentProfileDeleteConflictDialogProps = {
  conflict: AgentProfileDeleteConflict | null;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

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
  const hasHardBlockers = routingTiers.length > 0;
  const watchersByKind = groupWatchersByKind(watchers);
  const workspaces = useAppStore((s) => s.workspaces.items);
  const providers = useAppStore((s) => s.settingsAgents.items);

  return (
    <AlertDialog open={!!conflict} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {hasHardBlockers
              ? t("agents:cannotDeleteAgentProfile")
              : t("agents:deleteAgentProfileTitle")}
          </AlertDialogTitle>
          <AlertDialogDescription asChild>
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
              {hasHardBlockers ? (
                <p className="mt-2">{t("agents:changeTierMappingsFirst")}</p>
              ) : (
                <p className="mt-2">{t("agents:deleteAnywayConsequences")}</p>
              )}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          {hasHardBlockers ? null : (
            <AlertDialogAction
              onClick={onConfirm}
              className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {t("agents:deleteAnyway")}
            </AlertDialogAction>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
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
    <div className="mt-2">
      <p className="font-medium text-sm">{title}</p>
      <ul className="list-disc list-inside mt-1 space-y-0.5">
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
    <div className="mt-2">
      <p className="font-medium text-sm">{t("agents:conflictWatchersTitle")}</p>
      <ul className="list-disc list-inside mt-1 space-y-0.5">
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
    <div className="mt-2">
      <p className="font-medium text-sm">{t("agents:conflictTierMappingsTitle")}</p>
      <ul className="list-disc list-inside mt-1 space-y-0.5">
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
