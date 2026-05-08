"use client";

import { useEffect, useState } from "react";
import { formatDistanceToNow } from "date-fns";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import type { GitHubRateLimitInfo, GitHubRateLimitSnapshot } from "@/lib/types/github";

const RESOURCE_LABELS: Record<string, string> = {
  core: "REST",
  graphql: "GraphQL",
  search: "Search",
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
  const now = useTickNow(30_000);
  if (!info) return null;
  const snapshots = snapshotsFromInfo(info);
  if (snapshots.length === 0) return null;
  const exhausted = snapshots.filter((s) => isExhausted(s, now));

  return (
    <div className="space-y-2" data-testid="github-rate-limit-display">
      {exhausted.length > 0 && (
        <Alert variant="destructive" data-testid="github-rate-limit-exhausted">
          <IconAlertTriangle className="h-4 w-4" />
          <AlertDescription className="text-sm">
            GitHub API rate limit exhausted on{" "}
            {exhausted.map((s) => RESOURCE_LABELS[s.resource] ?? s.resource).join(", ")}. Background
            PR/issue checks are paused until the limit resets {formatReset(latestReset(exhausted))}.
          </AlertDescription>
        </Alert>
      )}
      <div className="text-xs text-muted-foreground flex flex-wrap gap-x-4 gap-y-1">
        {snapshots.map((snap) => {
          const label = RESOURCE_LABELS[snap.resource] ?? snap.resource;
          const limit = snap.limit > 0 ? snap.limit : "?";
          const reset = formatReset(snap);
          return (
            <span key={snap.resource} data-testid={`github-rate-limit-${snap.resource}`}>
              {label}: <strong>{snap.remaining}</strong>/{limit}
              {reset ? ` · resets ${reset}` : ""}
            </span>
          );
        })}
      </div>
    </div>
  );
}
