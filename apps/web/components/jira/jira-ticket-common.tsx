"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { formatDistanceToNow } from "date-fns";
import { IconLockExclamation } from "@tabler/icons-react";
import { Avatar, AvatarFallback, AvatarImage } from "@kandev/ui/avatar";
import { Button } from "@kandev/ui/button";
import { getJiraTicket, transitionJiraTicket } from "@/lib/api/domains/jira-api";
import type { JiraStatusCategory, JiraTicket } from "@/lib/types/jira";

// Matches PROJECT-123 anywhere in the string. Jira keys start with letters and
// include an uppercase prefix, followed by a dash and one or more digits.
const JIRA_KEY_RE = /\b[A-Z][A-Z0-9]+-\d+\b/;

export function extractJiraKey(title: string | undefined | null): string | null {
  if (!title) return null;
  const match = title.match(JIRA_KEY_RE);
  return match ? match[0] : null;
}

// Map Jira's statusCategory to Tailwind color classes. Jira returns one of
// three stable keys: "new" (to-do), "indeterminate" (in-progress), "done".
export function statusBadgeClass(category: JiraStatusCategory | undefined): string {
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

export function IconLabel({ icon, label }: { icon?: string; label?: string }) {
  if (!label) return <span className="text-muted-foreground">—</span>;
  return (
    <>
      {icon && (
        // Jira serves issuetype/priority icons from its CDN at fixed 16x16;
        // next/image would require per-host allowlisting for no real gain.
        // eslint-disable-next-line @next/next/no-img-element
        <img src={icon} alt="" className="h-3.5 w-3.5" />
      )}
      <span className="truncate">{label}</span>
    </>
  );
}

export type TicketState = {
  ticket: JiraTicket | null;
  loading: boolean;
  error: string | null;
  pendingTransition: string | null;
  load: () => Promise<void>;
  handleTransition: (id: string) => Promise<void>;
};

// Shared hook for loading a Jira ticket and applying transitions. Consumers
// pass `enabled` so the fetch only kicks off when the UI (popover/dialog) is
// visible. Re-runs `load` when workspace/ticket key change.
export function useTicketState(
  workspaceId: string,
  ticketKey: string,
  enabled: boolean,
): TicketState {
  const [ticket, setTicket] = useState<JiraTicket | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pendingTransition, setPendingTransition] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const t = await getJiraTicket(workspaceId, ticketKey);
      setTicket(t);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [workspaceId, ticketKey]);

  useEffect(() => {
    if (!enabled || !workspaceId || !ticketKey) return;
    let cancelled = false;
    async function run() {
      setTicket(null);
      setLoading(true);
      setError(null);
      try {
        const t = await getJiraTicket(workspaceId, ticketKey);
        if (!cancelled) setTicket(t);
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
  }, [enabled, workspaceId, ticketKey]);

  const handleTransition = useCallback(
    async (transitionId: string) => {
      setPendingTransition(transitionId);
      setError(null);
      try {
        await transitionJiraTicket(workspaceId, ticketKey, transitionId);
        await load();
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setPendingTransition(null);
      }
    },
    [workspaceId, ticketKey, load],
  );

  return { ticket, loading, error, pendingTransition, load, handleTransition };
}

// Backend wraps Jira responses as `jira api: status N: …`. 401/403 mean the
// session is invalid; 303 with "Step-up" means Atlassian wants re-auth.
const AUTH_STATUS_RE = /\bstatus (?:303|401|403)\b/i;
const STEP_UP_RE = /step-?up authentication/i;

export function isJiraAuthError(error: string): boolean {
  return AUTH_STATUS_RE.test(error) || STEP_UP_RE.test(error);
}

// Drops support URLs Atlassian inlines into 3xx response bodies — they're
// noise once the user has a clear CTA.
const URL_RE = /\bhttps?:\/\/\S+/g;

export function cleanJiraErrorMessage(error: string): string {
  return error.replace(URL_RE, "").replace(/\s+/g, " ").trim();
}

type JiraErrorMessageProps = {
  error: string;
  workspaceId?: string | null;
  /** Inline variant for cases where a ticket is already rendered above. */
  compact?: boolean;
};

export function JiraErrorMessage({ error, workspaceId, compact }: JiraErrorMessageProps) {
  const isAuth = isJiraAuthError(error);
  const settingsHref = workspaceId ? `/settings/workspace/${workspaceId}/jira` : "/settings";

  if (compact) {
    return (
      <div className="flex items-center gap-3 text-sm">
        <span className={isAuth ? "text-muted-foreground" : "text-destructive"}>
          {isAuth ? "Jira authentication required." : cleanJiraErrorMessage(error)}
        </span>
        {isAuth && (
          <Button asChild size="sm" variant="outline" className="cursor-pointer h-7 text-xs">
            <Link href={settingsHref}>Reconnect Jira</Link>
          </Button>
        )}
      </div>
    );
  }

  return (
    <div className="max-w-md text-center space-y-4">
      {isAuth ? (
        <>
          <IconLockExclamation className="h-10 w-10 mx-auto text-muted-foreground" />
          <div className="space-y-1.5">
            <h2 className="text-lg font-semibold">Jira authentication required</h2>
            <p className="text-sm text-muted-foreground">
              Your Jira session expired or needs step-up authentication. Reconnect to view this
              ticket.
            </p>
          </div>
          <Button asChild size="sm" className="cursor-pointer">
            <Link href={settingsHref}>Reconnect Jira</Link>
          </Button>
        </>
      ) : (
        <p className="text-sm text-destructive">{cleanJiraErrorMessage(error)}</p>
      )}
    </div>
  );
}
