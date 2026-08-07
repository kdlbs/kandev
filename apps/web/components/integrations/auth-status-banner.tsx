"use client";

import { useEffect, useState } from "react";
import { formatRelative } from "@/lib/i18n/formats";
import { IconAlertTriangle, IconCheck } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { useTranslation } from "react-i18next";

// AuthHealth captures everything every integration's config row tells us
// about the most-recent backend health probe. Each integration's settings
// page builds one of these from its own config shape.
export type IntegrationAuthHealth = {
  ok: boolean;
  error: string;
  checkedAt: Date | null;
};

// Re-render every 30s so "checked 1 minute ago" doesn't sit stale on a long-
// open settings tab. Exported so other settings status banners (e.g. workflow
// sync) can reuse the same self-refreshing relative-time pattern.
export function useTick(intervalMs: number) {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((n) => n + 1), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
}

function LastCheckedLabel({ checkedAt }: { checkedAt: Date | null }) {
  const { t } = useTranslation();
  useTick(30_000);
  if (!checkedAt) return null;
  return (
    <span className="text-xs text-muted-foreground ml-2">
      {/* `formatRelative` routes its buckets through i18next; date-fns'
          `formatDistanceToNow` would render English inside a translated
          sentence. Same call as workflow-sync-status-banner.tsx. */}
      {t("integrations:checkedRelative", {
        relative: formatRelative(checkedAt.toISOString()),
      })}
    </span>
  );
}

// IntegrationAuthStatusBanner renders the standard authenticated /
// authentication-failed / waiting alerts shown on every integration's
// settings page. Returns null when health is null (config not yet loaded or
// no secret configured) so the caller doesn't have to guard.
export function IntegrationAuthStatusBanner({ health }: { health: IntegrationAuthHealth | null }) {
  const { t } = useTranslation();
  if (!health) return null;
  if (!health.checkedAt) {
    return (
      <Alert data-testid="integration-auth-status-banner" data-state="waiting">
        <AlertDescription className="text-sm">
          {t("integrations:waitingForTheNextBackendHealth")}
        </AlertDescription>
      </Alert>
    );
  }
  if (health.ok) {
    return (
      <Alert
        data-testid="integration-auth-status-banner"
        data-state="ok"
        className="border-green-500/40 bg-green-500/10 dark:border-green-400/30 dark:bg-green-400/10"
      >
        <IconCheck className="h-4 w-4 text-green-600 dark:text-green-400" />
        <AlertDescription className="text-sm font-medium">
          {t("integrations:authenticated")}
          <LastCheckedLabel checkedAt={health.checkedAt} />
        </AlertDescription>
      </Alert>
    );
  }
  return (
    <Alert data-testid="integration-auth-status-banner" data-state="failed" variant="destructive">
      <IconAlertTriangle className="h-4 w-4" />
      <AlertDescription className="text-sm">
        {t("integrations:authenticationFailed", {
          error: health.error || t("integrations:unknownError"),
        })}
        <LastCheckedLabel checkedAt={health.checkedAt} />
      </AlertDescription>
    </Alert>
  );
}
