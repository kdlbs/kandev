"use client";

import { useCallback, useEffect, useState } from "react";
import { formatDistanceToNow } from "date-fns";
import { Avatar, AvatarFallback, AvatarImage } from "@kandev/ui/avatar";
import { getLinearIssue, setLinearIssueState } from "@/lib/api/domains/linear-api";
import type { LinearIssue, LinearStateCategory } from "@/lib/types/linear";
import { IntegrationAuthErrorMessage } from "@/components/integrations/auth-error-message";

// Matches Linear identifiers like ENG-123. Linear team keys are always
// uppercase and 1+ chars; we require a leading capital letter to avoid catching
// random UUID fragments or version strings (v1-2). Anchored on word boundaries
// so we can extract from "ENG-12: fix login".
export const LINEAR_KEY_RE = /\b[A-Z][A-Z0-9]*-\d+\b/;

export function extractLinearKey(title: string | undefined | null): string | null {
  if (!title) return null;
  const match = title.match(LINEAR_KEY_RE);
  return match ? match[0] : null;
}

// Map Linear's stateCategory to Tailwind colour classes — same three buckets
// the Jira integration uses so status pills look consistent.
export function stateBadgeClass(category: LinearStateCategory | undefined): string {
  switch (category) {
    case "done":
      return "bg-green-500/15 text-green-700 dark:text-green-400 border-green-500/30";
    case "indeterminate":
      return "bg-amber-500/15 text-amber-700 dark:text-amber-400 border-amber-500/30";
    case "new":
      return "bg-blue-500/15 text-blue-700 dark:text-blue-400 border-blue-500/30";
    default:
      return "";
  }
}

export function formatRelative(iso: string | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return formatDistanceToNow(d, { addSuffix: true });
}

export function PersonCell({ name, avatar }: { name?: string; avatar?: string }) {
  if (!name) return <span className="text-muted-foreground">Unassigned</span>;
  return (
    <>
      <Avatar size="sm" className="size-5">
        {avatar && <AvatarImage src={avatar} alt={name} />}
        <AvatarFallback className="text-[10px]">{name.charAt(0)}</AvatarFallback>
      </Avatar>
      <span className="truncate">{name}</span>
    </>
  );
}

// Linear priority enum: 0=none, 1=urgent, 2=high, 3=medium, 4=low. The label
// the API returns is already humanised — we just keep the colour cue.
export function priorityClass(priority: number | undefined): string {
  switch (priority) {
    case 1:
      return "text-red-600 dark:text-red-400";
    case 2:
      return "text-orange-600 dark:text-orange-400";
    case 3:
      return "text-amber-600 dark:text-amber-400";
    case 4:
      return "text-muted-foreground";
    default:
      return "text-muted-foreground";
  }
}

export type IssueState = {
  issue: LinearIssue | null;
  loading: boolean;
  error: string | null;
  pendingState: string | null;
  load: () => Promise<void>;
  handleStateChange: (stateId: string) => Promise<void>;
};

// Shared hook for loading a Linear issue and changing its workflow state.
// Mirrors the Jira useTicketState hook.
export function useIssueState(
  workspaceId: string,
  identifier: string,
  enabled: boolean,
): IssueState {
  const [issue, setIssue] = useState<LinearIssue | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pendingState, setPendingState] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const i = await getLinearIssue(identifier);
      setIssue(i);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [identifier]);

  useEffect(() => {
    if (!enabled || !workspaceId || !identifier) return;
    let cancelled = false;
    async function run() {
      setIssue(null);
      setLoading(true);
      setError(null);
      try {
        const i = await getLinearIssue(identifier);
        if (!cancelled) setIssue(i);
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void run();
    return () => {
      cancelled = true;
    };
  }, [enabled, workspaceId, identifier]);

  const handleStateChange = useCallback(
    async (stateId: string) => {
      if (!issue) return;
      setPendingState(stateId);
      setError(null);
      try {
        await setLinearIssueState(issue.id, stateId);
        await load();
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setPendingState(null);
      }
    },
    [issue, load],
  );

  return { issue, loading, error, pendingState, load, handleStateChange };
}

// Backend wraps Linear errors as `linear api: status N: …`. 401/403 mean the
// API key is invalid; everything else is propagated verbatim.
const AUTH_STATUS_RE = /\bstatus (?:401|403)\b/i;

export function isLinearAuthError(error: string): boolean {
  return AUTH_STATUS_RE.test(error);
}

export { cleanIntegrationErrorMessage as cleanLinearErrorMessage } from "@/components/integrations/auth-error-message";

type LinearErrorMessageProps = {
  error: string;
  compact?: boolean;
};

export function LinearErrorMessage({ error, compact }: LinearErrorMessageProps) {
  return (
    <IntegrationAuthErrorMessage
      error={error}
      name="Linear"
      reconnectHref="/settings/integrations/linear"
      isAuthError={isLinearAuthError}
      authErrorBody="Your Linear API key is invalid or has been revoked. Reconnect to view this issue."
      compact={compact}
    />
  );
}
