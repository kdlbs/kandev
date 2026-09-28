"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { IconLoader2 } from "@tabler/icons-react";
import type { SSHReachabilityRecord, SSHReachabilityReason } from "@/lib/types/http-ssh";
import { formatRelative } from "@/lib/i18n/formats";
import { useSSHReachability } from "@/hooks/domains/settings/use-ssh-reachability";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { settingsActionClassName } from "@/components/settings/settings-control";

export interface SSHReachabilityCardProps {
  executorId: string;
}

const STALE_INTERVAL_MULTIPLIER = 3;

/**
 * A record older than three probe intervals is stale rather than current.
 * Disabled probing has no cadence to measure against, and a never-probed
 * record has nothing to be stale relative to, so both read as not-stale.
 */
export function isReachabilityStale(
  checkedAt: string | null,
  probeIntervalSeconds: number,
  probingEnabled: boolean,
  now: number = Date.now(),
): boolean {
  if (!probingEnabled || !checkedAt || probeIntervalSeconds <= 0) return false;
  const checkedMs = Date.parse(checkedAt);
  if (Number.isNaN(checkedMs)) return false;
  return now - checkedMs > probeIntervalSeconds * 1000 * STALE_INTERVAL_MULTIPLIER;
}

export function reachabilityReasonKey(reason: SSHReachabilityReason): string | null {
  switch (reason) {
    case "config":
      return "executors:sshReachabilityReasonConfig";
    case "timeout":
      return "executors:sshReachabilityReasonTimeout";
    case "host_key":
      return "executors:sshReachabilityReasonHostKey";
    case "auth":
      return "executors:sshReachabilityReasonAuth";
    case "network":
      return "executors:sshReachabilityReasonNetwork";
    case "unknown":
      return "executors:sshReachabilityReasonUnknown";
    default:
      return null;
  }
}

export function reachabilityStateKey(state: SSHReachabilityRecord["state"]): string {
  switch (state) {
    case "reachable":
      return "executors:sshReachabilityStateReachable";
    case "unreachable":
      return "executors:sshReachabilityStateUnreachable";
    default:
      return "executors:sshReachabilityStateUnknown";
  }
}

/**
 * Renders the most recent SSH reachability record for one executor: state,
 * probed host, failure reason/message, consecutive-failure count, and the
 * age of the last completed and last successful probe. Never fetches
 * directly into local-only state — every fetch result is written through the
 * store so a live executor.reachability.changed push and this card's own
 * refresh share one reconciliation path.
 */
export function SSHReachabilityCard({ executorId }: SSHReachabilityCardProps) {
  const { t } = useTranslation();
  const { record, loadError, probing, probeNow, now } = useSSHReachability(executorId);

  return (
    <SettingsCard data-testid="ssh-reachability-card">
      <SettingsCardHeader
        title={t("executors:sshReachabilityTitle")}
        description={t("executors:sshReachabilityDescription")}
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={() => void probeNow()}
            disabled={probing}
            data-testid="ssh-reachability-probe-now"
            className={settingsActionClassName("cursor-pointer")}
          >
            {probing ? <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
            {t("executors:sshReachabilityProbeNow")}
          </Button>
        }
      />
      <CardContent>
        <ReachabilityBody record={record} loadError={loadError} now={now} />
      </CardContent>
    </SettingsCard>
  );
}

function ReachabilityBody({
  record,
  loadError,
  now,
}: {
  record: SSHReachabilityRecord | undefined;
  loadError: boolean;
  now: number;
}) {
  const { t } = useTranslation();
  if (!record) {
    if (loadError) {
      return (
        <p data-testid="ssh-reachability-not-known" className="text-sm text-muted-foreground">
          {t("executors:sshReachabilityLoadFailed")}
        </p>
      );
    }
    return null;
  }
  const stale = isReachabilityStale(
    record.checked_at,
    record.probe_interval_seconds,
    record.probing_enabled,
    now,
  );
  const reasonKey = record.reason ? reachabilityReasonKey(record.reason) : null;
  return (
    <div className="space-y-2 text-sm">
      <div className="flex items-center gap-2">
        <StateBadge state={record.state} />
        {stale ? (
          <Badge
            data-testid="ssh-reachability-stale"
            variant="outline"
            className="border-amber-500/30 bg-amber-500/10 text-amber-700"
          >
            {t("executors:sshReachabilityStale")}
          </Badge>
        ) : null}
      </div>
      {record.host ? (
        <p data-testid="ssh-reachability-host">
          {t("executors:sshReachabilityHost", { host: record.host })}
        </p>
      ) : null}
      {record.state === "unreachable" && reasonKey ? (
        <p data-testid="ssh-reachability-reason">{t(reasonKey)}</p>
      ) : null}
      {record.state === "unreachable" && record.message ? (
        <p data-testid="ssh-reachability-message" className="text-muted-foreground">
          {record.message}
        </p>
      ) : null}
      <p data-testid="ssh-reachability-failures">
        {t("executors:sshReachabilityConsecutiveFailures", { count: record.consecutive_failures })}
      </p>
      <p data-testid="ssh-reachability-age">
        {record.checked_at
          ? t("executors:sshReachabilityAgeLastProbe", { age: formatRelative(record.checked_at) })
          : t("executors:sshReachabilityNeverProbed")}
      </p>
      <p data-testid="ssh-reachability-last-success">
        {record.last_success_at
          ? t("executors:sshReachabilityAgeLastSuccess", {
              age: formatRelative(record.last_success_at),
            })
          : t("executors:sshReachabilityNoSuccess")}
      </p>
      {!record.probing_enabled ? (
        <p data-testid="ssh-reachability-probing-off" className="text-muted-foreground">
          {t("executors:sshReachabilityProbingOff")}
        </p>
      ) : null}
    </div>
  );
}

const STATE_BADGE_CLASSNAMES: Record<SSHReachabilityRecord["state"], string> = {
  reachable: "border-green-500/30 bg-green-500/10 text-green-700",
  unreachable: "border-red-500/30 bg-red-500/10 text-red-700",
  unknown: "border-muted-foreground/30 bg-muted text-muted-foreground",
};

function StateBadge({ state }: { state: SSHReachabilityRecord["state"] }) {
  const { t } = useTranslation();
  const variantClass = STATE_BADGE_CLASSNAMES[state];
  return (
    <Badge data-testid="ssh-reachability-state" variant="outline" className={variantClass}>
      {t(reachabilityStateKey(state))}
    </Badge>
  );
}
