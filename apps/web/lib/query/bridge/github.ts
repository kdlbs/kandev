import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import type {
  GitHubRateLimitUpdate,
  TaskPR,
  GitHubRateLimitInfo,
  GitHubRateLimitSnapshot,
} from "@/lib/types/github";
import type { GitHubStatusResponse } from "@/lib/types/github";
import { qk } from "@/lib/query/keys";
import { wrapBridgeHandler } from "./index";

// ---------------------------------------------------------------------------
// Rate-limit helpers — patch incoming snapshots into the cached GitHub status.
// ---------------------------------------------------------------------------

function applyRateLimitUpdate(
  existing: GitHubStatusResponse | undefined,
  update: GitHubRateLimitUpdate,
): GitHubStatusResponse | undefined {
  if (!existing) return existing;
  const rateLimit: GitHubRateLimitInfo = { ...(existing.rate_limit ?? {}) };
  for (const snap of update.snapshots) {
    (rateLimit as Record<string, GitHubRateLimitSnapshot>)[snap.resource] = snap;
  }
  return { ...existing, rate_limit: rateLimit };
}

// ---------------------------------------------------------------------------
// TaskPR upsert helper (shared shape with upsertTaskPRIntoCaches in use-task-pr)
//
// Upserts by (repository_id, pr_number) so multi-branch tasks can hold N PRs
// on the same repo as siblings. Keying on repository_id alone collapses every
// PR for that repo onto one slot — the second WS event silently overwrites the
// first and the UI shows only the most-recent PR. Legacy rows without a
// repository_id match on the empty key + pr_number, preserving prior single-PR
// semantics for single-repo tasks.
// ---------------------------------------------------------------------------

function upsertTaskPR(
  existing: Record<string, TaskPR[]> | undefined,
  pr: TaskPR,
): Record<string, TaskPR[]> {
  const byTaskId = existing ?? {};
  const current = byTaskId[pr.task_id];
  const list = Array.isArray(current) ? current : [];
  const repoKey = pr.repository_id ?? "";
  const idx = list.findIndex(
    (p) => (p.repository_id ?? "") === repoKey && p.pr_number === pr.pr_number,
  );
  const next = idx >= 0 ? list.map((p, i) => (i === idx ? pr : p)) : [...list, pr];
  return { ...byTaskId, [pr.task_id]: next };
}

// ---------------------------------------------------------------------------
// Bridge registrar
// ---------------------------------------------------------------------------

/**
 * Registers WS handlers for the GitHub domain into the TanStack Query cache.
 *
 * GitHub server state is TQ-only (the Zustand mirror was removed); these
 * handlers are the sole apply path, writing via queryClient.setQueryData with
 * immutable functional updaters.
 *
 * Events handled:
 *   github.task_pr.updated  — upsert PR into workspace PR cache
 *   github.rate_limit.updated — apply rate-limit snapshot updates into status
 *
 * Returns a cleanup function that removes all registered handlers.
 */
export function registerGithubBridge(ws: WebSocketClient, queryClient: QueryClient): () => void {
  // github.task_pr.updated — push into the workspace PR map.
  // The PR contains task_id but not workspace_id, so we use qk.github.prs("all")
  // as a global aggregation key, then individual workspace caches are updated
  // when the PR's workspace context is known. We iterate all cached workspace
  // PR queries and upsert the PR into the matching one.
  const unsubTaskPR = ws.on(
    "github.task_pr.updated",
    wrapBridgeHandler(queryClient, "github.task_pr.updated", (message) => {
      const pr = message.payload as TaskPR;
      if (!pr.task_id) return;

      // Update every cached workspace PR query that we know about.
      // Since we don't know which workspace the PR belongs to from the event
      // alone, we scan all active queries with prefix ["github"] and update
      // any that have cached PR data (task_prs map).
      const queries = queryClient.getQueryCache().findAll({
        predicate: (q) => {
          const key = q.queryKey as unknown[];
          return key[0] === "github" && key[2] === "prs";
        },
      });

      if (queries.length > 0) {
        for (const q of queries) {
          queryClient.setQueryData<{ task_prs: Record<string, TaskPR[]> }>(q.queryKey, (prev) => {
            if (!prev) return prev;
            return { ...prev, task_prs: upsertTaskPR(prev.task_prs, pr) };
          });
        }
      }
    }),
  );

  // github.rate_limit.updated — patch rate_limit into the GitHub status cache.
  const unsubRateLimit = ws.on(
    "github.rate_limit.updated",
    wrapBridgeHandler(queryClient, "github.rate_limit.updated", (message) => {
      const update = message.payload as GitHubRateLimitUpdate;
      if (!update?.snapshots?.length) return;

      queryClient.setQueryData<GitHubStatusResponse>(qk.github.status(), (prev) =>
        applyRateLimitUpdate(prev, update),
      );
    }),
  );

  return () => {
    unsubTaskPR();
    unsubRateLimit();
  };
}
