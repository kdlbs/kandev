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

// ---------------------------------------------------------------------------
// Rate-limit helpers (mirror applyGitHubRateLimitUpdate from github-slice.ts)
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
// TaskPR upsert helper (mirrors setTaskPR logic from github-slice.ts)
//
// Upserts by repository_id so multi-repo PRs coexist for the same task.
// For legacy rows without a repository_id, match on the empty key.
// ---------------------------------------------------------------------------

function upsertTaskPR(
  existing: Record<string, TaskPR[]> | undefined,
  pr: TaskPR,
): Record<string, TaskPR[]> {
  const byTaskId = existing ?? {};
  const current = byTaskId[pr.task_id];
  const list = Array.isArray(current) ? current : [];
  const repoKey = pr.repository_id ?? "";
  const idx = list.findIndex((p) => (p.repository_id ?? "") === repoKey);
  const next = idx >= 0 ? list.map((p, i) => (i === idx ? pr : p)) : [...list, pr];
  return { ...byTaskId, [pr.task_id]: next };
}

// ---------------------------------------------------------------------------
// Bridge registrar
// ---------------------------------------------------------------------------

/**
 * Registers WS handlers for the GitHub domain into the TanStack Query cache.
 *
 * Mirrors lib/ws/handlers/github.ts 1:1, replacing store mutations with
 * queryClient.setQueryData using immutable functional updaters.
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
  const unsubTaskPR = ws.on("github.task_pr.updated", (message) => {
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
  });

  // github.rate_limit.updated — patch rate_limit into the GitHub status cache.
  const unsubRateLimit = ws.on("github.rate_limit.updated", (message) => {
    const update = message.payload as GitHubRateLimitUpdate;
    if (!update?.snapshots?.length) return;

    queryClient.setQueryData<GitHubStatusResponse>(qk.github.status(), (prev) =>
      applyRateLimitUpdate(prev, update),
    );
  });

  return () => {
    unsubTaskPR();
    unsubRateLimit();
  };
}
