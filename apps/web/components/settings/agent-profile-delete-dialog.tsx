"use client";

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

const WATCHER_KIND_LABELS: Record<WatcherReference["kind"], string> = {
  linear: "Linear",
  jira: "Jira",
  github_issue: "GitHub Issues",
  github_review: "GitHub PR Reviews",
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
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete agent profile?</AlertDialogTitle>
          <AlertDialogDescription>
            This will permanently delete this profile. This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Delete
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
            {hasHardBlockers ? "Cannot delete agent profile" : "Delete agent profile?"}
          </AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>This profile is currently in use. Deleting it will affect the following:</p>
              <SessionConflictSection title="Tasks:" sessions={tasks} fallback="Untitled task" />
              <SessionConflictSection
                title="Quick Chats:"
                sessions={quickChats}
                fallback="Untitled quick chat"
              />
              <WatcherConflictSection watchersByKind={watchersByKind} />
              <RoutingTierConflictSection
                routingTiers={routingTiers}
                workspaceLabels={new Map(workspaces.map((w) => [w.id, w.name]))}
                providerLabels={new Map(providers.map((p) => [p.id, p.name]))}
              />
              {hasHardBlockers ? (
                <p className="mt-2">
                  Change these workspace tier mappings before deleting this profile.
                </p>
              ) : (
                <p className="mt-2">
                  These sessions will no longer be able to use this profile and the listed watchers
                  will be disabled. This action cannot be undone.
                </p>
              )}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          {hasHardBlockers ? null : (
            <AlertDialogAction
              onClick={onConfirm}
              className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete Anyway
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
  const entries = Object.entries(watchersByKind);
  if (entries.length === 0) return null;
  return (
    <div className="mt-2">
      <p className="font-medium text-sm">Watchers (will be disabled):</p>
      <ul className="list-disc list-inside mt-1 space-y-0.5">
        {entries.map(([kind, items]) => (
          <li key={kind} className="text-sm">
            <span className="font-medium">
              {WATCHER_KIND_LABELS[kind as WatcherReference["kind"]] ?? kind}:
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
  if (routingTiers.length === 0) return null;
  return (
    <div className="mt-2">
      <p className="font-medium text-sm">Workspace tier mappings:</p>
      <ul className="list-disc list-inside mt-1 space-y-0.5">
        {routingTiers.map((ref) => (
          <li key={`${ref.workspace_id}-${ref.provider_id}-${ref.tier}`} className="text-sm">
            <span className="font-medium">{formatRoutingTier(ref.tier)}</span> tier in{" "}
            {formatLookupLabel(workspaceLabels, ref.workspace_id)} for{" "}
            {formatLookupLabel(providerLabels, ref.provider_id)}
          </li>
        ))}
      </ul>
    </div>
  );
}

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
