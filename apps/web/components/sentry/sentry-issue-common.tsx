"use client";

import { formatDistanceToNow } from "date-fns";
import { Badge } from "@kandev/ui/badge";
import type { SentryIssue, SentryLevel, SentryStatus } from "@/lib/types/sentry";
import { IntegrationAuthErrorMessage } from "@/components/integrations/auth-error-message";

// Sentry short IDs look like "PROJ-123" — alphanumeric uppercase project slug
// followed by a numeric counter. Underscores and hyphens are allowed in the
// project slug part.
export const SENTRY_SHORT_ID_RE = /^[A-Z][A-Z0-9_-]*-\d+$/;
export const SENTRY_SHORT_ID_EXTRACT_RE = /\b[A-Z][A-Z0-9_-]*-\d+\b/;

export function extractSentryShortId(input: string | undefined | null): string | null {
  if (!input) return null;
  const match = input.toUpperCase().match(SENTRY_SHORT_ID_EXTRACT_RE);
  return match ? match[0] : null;
}

export function levelBadgeClass(level: SentryLevel | undefined): string {
  switch (level) {
    case "fatal":
      return "bg-red-500/15 text-red-700 dark:text-red-400 border-red-500/30";
    case "error":
      return "bg-orange-500/15 text-orange-700 dark:text-orange-400 border-orange-500/30";
    case "warning":
      return "bg-amber-500/15 text-amber-700 dark:text-amber-400 border-amber-500/30";
    case "info":
      return "bg-blue-500/15 text-blue-700 dark:text-blue-400 border-blue-500/30";
    case "debug":
      return "bg-muted text-muted-foreground border-muted-foreground/30";
    default:
      return "";
  }
}

export function statusBadgeClass(status: SentryStatus | undefined): string {
  switch (status) {
    case "unresolved":
      return "bg-red-500/10 text-red-700 dark:text-red-400 border-red-500/30";
    case "resolved":
      return "bg-green-500/15 text-green-700 dark:text-green-400 border-green-500/30";
    case "ignored":
      return "bg-muted text-muted-foreground border-muted-foreground/30";
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

export function SentryIssueRow({ issue }: { issue: SentryIssue }) {
  const lastSeen = formatRelative(issue.lastSeen);
  return (
    <div className="space-y-1.5 rounded-md border bg-background px-3 py-2.5">
      <div className="flex items-center gap-2 flex-wrap">
        <Badge
          variant="outline"
          className={`text-[10px] uppercase px-1.5 py-0 ${levelBadgeClass(issue.level)}`}
        >
          {issue.level}
        </Badge>
        <Badge
          variant="outline"
          className={`text-[10px] uppercase px-1.5 py-0 ${statusBadgeClass(issue.status)}`}
        >
          {issue.status}
        </Badge>
        <span className="text-xs font-mono text-muted-foreground">{issue.shortId}</span>
        {issue.projectName && (
          <span className="text-xs text-muted-foreground">· {issue.projectName}</span>
        )}
      </div>
      <div className="text-sm font-medium leading-snug">{issue.title}</div>
      {issue.culprit && (
        <div className="text-xs text-muted-foreground font-mono truncate">{issue.culprit}</div>
      )}
      <SentryIssueMeta issue={issue} lastSeen={lastSeen} />
    </div>
  );
}

function SentryIssueMeta({ issue, lastSeen }: { issue: SentryIssue; lastSeen: string }) {
  return (
    <div className="flex items-center gap-3 text-xs text-muted-foreground">
      {issue.count != null && issue.count !== "" && <span>events {issue.count}</span>}
      {typeof issue.userCount === "number" && <span>users {issue.userCount}</span>}
      {lastSeen && <span title={issue.lastSeen}>Last seen {lastSeen}</span>}
    </div>
  );
}

// Backend wraps Sentry errors as `sentry api: status N: …`. 401/403 mean the
// token is invalid; everything else is propagated verbatim.
const AUTH_STATUS_RE = /\bstatus (?:401|403)\b/i;

export function isSentryAuthError(error: string): boolean {
  return AUTH_STATUS_RE.test(error);
}

type SentryErrorMessageProps = {
  error: string;
  compact?: boolean;
};

export function SentryErrorMessage({ error, compact }: SentryErrorMessageProps) {
  return (
    <IntegrationAuthErrorMessage
      error={error}
      name="Sentry"
      reconnectHref="/settings/integrations/sentry"
      isAuthError={isSentryAuthError}
      authErrorBody="Your Sentry auth token is invalid or has been revoked. Reconnect to view this issue."
      compact={compact}
    />
  );
}
