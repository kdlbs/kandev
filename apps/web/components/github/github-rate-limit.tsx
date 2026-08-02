"use client";

import { useEffect, useState } from "react";
import { formatDistanceToNow } from "date-fns";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import type { GitHubRateLimitInfo, GitHubRateLimitSnapshot } from "@/lib/types/github";
import { GitHubAccessHelp } from "./github-access-help";
import { useTranslation } from "react-i18next";
// Locale-aware digit grouping; `toLocaleString("en-US")` pinned it to English.
import { formatNumber } from "@/lib/i18n/formats";

// Keyed by GitHub's rate-limit resource name, which is wire data. Catalog keys
// rather than `t()` calls, because this is module scope (see docs/i18n.md).
const RESOURCE_LABELS: Record<string, { labelKey: string; unitKey: string }> = {
  core: { labelKey: "github:apiRateLimit", unitKey: "github:rateLimitUnitRequests" },
  graphql: { labelKey: "github:graphqlQueryLimit", unitKey: "github:rateLimitUnitPoints" },
  search: { labelKey: "github:searchLimit", unitKey: "github:rateLimitUnitRequests" },
};

// useTickNow re-renders every intervalMs and exposes a stable now-value so the
// render pass stays pure (Date.now() in the render body would be impure).
function useTickNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

function isExhausted(snap: GitHubRateLimitSnapshot, now: number): boolean {
  if (snap.remaining > 0) return false;
  const reset = new Date(snap.reset_at).getTime();
  return Number.isFinite(reset) && reset > now;
}

function formatReset(snap: GitHubRateLimitSnapshot): string {
  const reset = new Date(snap.reset_at);
  if (!Number.isFinite(reset.getTime())) return "";
  return formatDistanceToNow(reset, { addSuffix: true });
}

// latestReset returns the snapshot whose reset_at is furthest in the future.
// When multiple buckets are exhausted, background checks remain paused until
// the last one recovers, so the alert must anchor on that bucket — anchoring
// on snapshots[0] would understate the pause window.
function latestReset(snaps: GitHubRateLimitSnapshot[]): GitHubRateLimitSnapshot {
  return snaps.reduce((latest, s) =>
    new Date(s.reset_at).getTime() > new Date(latest.reset_at).getTime() ? s : latest,
  );
}

function snapshotsFromInfo(info: GitHubRateLimitInfo): GitHubRateLimitSnapshot[] {
  const out: GitHubRateLimitSnapshot[] = [];
  if (info.core) out.push(info.core);
  if (info.graphql) out.push(info.graphql);
  if (info.search) out.push(info.search);
  return out;
}

export function GitHubRateLimitDisplay({ info }: { info?: GitHubRateLimitInfo }) {
  const { t } = useTranslation();
  const now = useTickNow(30_000);
  if (!info) return null;
  const snapshots = snapshotsFromInfo(info);
  if (snapshots.length === 0) return null;
  const exhausted = snapshots.filter((s) => isExhausted(s, now));

  return (
    <GitHubAccessHelp
      label={t("github:showGithubApiLimits")}
      title={t("github:githubApiLimits")}
      description={t("github:currentGithubApiRateAndQuery")}
      content={<RateLimitDetails snapshots={snapshots} exhausted={exhausted} />}
    />
  );
}

function RateLimitDetails({
  snapshots,
  exhausted,
}: {
  snapshots: GitHubRateLimitSnapshot[];
  exhausted: GitHubRateLimitSnapshot[];
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" data-testid="github-rate-limit-display">
      {exhausted.length > 0 && (
        <Alert variant="destructive" data-testid="github-rate-limit-exhausted">
          <IconAlertTriangle className="h-4 w-4" />
          <AlertDescription className="text-xs">
            {t("github:githubApiAccessIsExhaustedFor", {
              resources: exhausted
                .map((snapshot) =>
                  RESOURCE_LABELS[snapshot.resource]
                    ? t(RESOURCE_LABELS[snapshot.resource].labelKey)
                    : snapshot.resource,
                )
                .join(", "),
              reset: formatReset(latestReset(exhausted)),
            })}
          </AlertDescription>
        </Alert>
      )}
      <div className="space-y-1 text-xs text-muted-foreground">
        {snapshots.map((snapshot) => {
          const resource = RESOURCE_LABELS[snapshot.resource];
          // An unknown resource name is wire data, so it is shown verbatim.
          const label = resource ? t(resource.labelKey) : snapshot.resource;
          const unit = t(resource?.unitKey ?? "github:rateLimitUnitRequests");
          const limit =
            snapshot.limit > 0 ? formatNumber(snapshot.limit) : t("github:rateLimitUnknown");
          const reset = formatReset(snapshot);
          return (
            <p key={snapshot.resource} data-testid={`github-rate-limit-${snapshot.resource}`}>
              <span className="font-medium text-foreground">{label}:</span>{" "}
              {t("github:rateLimitRemaining", {
                remaining: formatNumber(snapshot.remaining),
                limit,
                unit,
              })}
              {reset ? t("github:resets", { reset }) : ""}
            </p>
          );
        })}
      </div>
    </div>
  );
}
