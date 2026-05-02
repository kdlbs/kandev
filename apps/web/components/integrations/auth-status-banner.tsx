"use client";

import { useEffect, useState } from "react";
import { formatDistanceToNow } from "date-fns";
import { IconAlertTriangle, IconCheck } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";

// AuthHealth captures everything every integration's config row tells us
// about the most-recent backend health probe. Each integration's settings
// page builds one of these from its own config shape.
export type IntegrationAuthHealth = {
  ok: boolean;
  error: string;
  checkedAt: Date | null;
};

// Re-render every 30s so "checked 1 minute ago" doesn't sit stale on a long-
// open settings tab.
function useTick(intervalMs: number) {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((n) => n + 1), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
}

function LastCheckedLabel({ checkedAt }: { checkedAt: Date | null }) {
  useTick(30_000);
  if (!checkedAt) return null;
  return (
    <span className="text-xs text-muted-foreground ml-2">
      · checked {formatDistanceToNow(checkedAt, { addSuffix: true })}
    </span>
  );
}

// IntegrationAuthStatusBanner renders the standard authenticated /
// authentication-failed / waiting alerts shown on every integration's
// settings page. Returns null when health is null (config not yet loaded or
// no secret configured) so the caller doesn't have to guard.
export function IntegrationAuthStatusBanner({ health }: { health: IntegrationAuthHealth | null }) {
  if (!health) return null;
  if (!health.checkedAt) {
    return (
      <Alert>
        <AlertDescription className="text-sm">
          Waiting for the next backend health check…
        </AlertDescription>
      </Alert>
    );
  }
  if (health.ok) {
    return (
      <Alert className="border-green-500/40 bg-green-500/10 dark:border-green-400/30 dark:bg-green-400/10">
        <IconCheck className="h-4 w-4 text-green-600 dark:text-green-400" />
        <AlertDescription className="text-sm font-medium">
          Authenticated
          <LastCheckedLabel checkedAt={health.checkedAt} />
        </AlertDescription>
      </Alert>
    );
  }
  return (
    <Alert variant="destructive">
      <IconAlertTriangle className="h-4 w-4" />
      <AlertDescription className="text-sm">
        Authentication failed: {health.error || "unknown error"}
        <LastCheckedLabel checkedAt={health.checkedAt} />
      </AlertDescription>
    </Alert>
  );
}
