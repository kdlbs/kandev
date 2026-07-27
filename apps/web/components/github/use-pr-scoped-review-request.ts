import { useCallback, useEffect, useSyncExternalStore } from "react";
import { requestPRReviewers } from "@/lib/api/domains/github-review-api";
import type { PRReview, RequestedReviewer, TaskPR } from "@/lib/types/github";

type Toast = (message: {
  title?: string;
  description: string;
  variant: "error" | "success";
}) => void;

type ReviewRequestOptions = {
  workspaceId: string | null;
  requestedReviewers: RequestedReviewer[];
  reviews: PRReview[];
  refresh: () => void;
  toast: Toast;
};

type ReviewBaseline = Pick<PRReview, "id" | "author" | "created_at" | "state">;

type RequestEntry = {
  identity: string;
  reviewer: string;
  baseline: ReviewBaseline | null;
  status: "requesting" | "optimistic";
  operationId: number;
  expiresAt: number | null;
};

const ENTRY_TTL_MS = 5 * 60 * 1000;
const entries = new Map<string, RequestEntry>();
const listeners = new Set<() => void>();
let revision = 0;
let nextOperationId = 0;
let expiryTimer: ReturnType<typeof setTimeout> | undefined;

function normalizeLogin(login: string) {
  return login.trim().toLowerCase();
}

function normalizeWorkspaceId(workspaceId: string) {
  return workspaceId.trim().toLowerCase();
}

function prIdentity(workspaceId: string | null, taskPR: TaskPR) {
  if (!workspaceId) return null;
  const normalizedWorkspaceId = normalizeWorkspaceId(workspaceId);
  if (!normalizedWorkspaceId) return null;
  return `${normalizedWorkspaceId}:${taskPR.owner.toLowerCase()}:${taskPR.repo.toLowerCase()}:${taskPR.pr_number}`;
}

function entryKey(identity: string, reviewer: string) {
  return `${identity}:${normalizeLogin(reviewer)}`;
}

function notify() {
  revision += 1;
  listeners.forEach((listener) => listener());
}

function pruneExpired(now = Date.now()) {
  let removed = false;
  for (const [key, entry] of entries) {
    if (entry.status === "optimistic" && entry.expiresAt !== null && entry.expiresAt <= now) {
      entries.delete(key);
      removed = true;
    }
  }
  return removed;
}

function rescheduleExpiryTimer() {
  if (expiryTimer !== undefined) {
    clearTimeout(expiryTimer);
    expiryTimer = undefined;
  }
  if (typeof window === "undefined") return;
  const nextExpiry = Array.from(entries.values()).reduce<number | null>((nearest, entry) => {
    if (entry.status !== "optimistic" || entry.expiresAt === null) return nearest;
    return nearest === null || entry.expiresAt < nearest ? entry.expiresAt : nearest;
  }, null);
  if (nextExpiry === null) return;
  expiryTimer = setTimeout(
    () => {
      expiryTimer = undefined;
      if (pruneExpired()) notify();
      rescheduleExpiryTimer();
    },
    Math.max(0, nextExpiry - Date.now()),
  );
}

function latestReview(reviews: PRReview[], reviewer: string) {
  const normalizedReviewer = normalizeLogin(reviewer);
  return reviews
    .filter((review) => normalizeLogin(review.author) === normalizedReviewer)
    .reduce<PRReview | null>((latest, review) => {
      if (!latest) return review;
      const reviewTime = new Date(review.created_at).getTime();
      const latestTime = new Date(latest.created_at).getTime();
      return reviewTime > latestTime || (reviewTime === latestTime && review.id > latest.id)
        ? review
        : latest;
    }, null);
}

function supersedesBaseline(review: PRReview | null, baseline: ReviewBaseline | null) {
  if (!review || !baseline || review.state === "DISMISSED") return false;
  const reviewTime = new Date(review.created_at).getTime();
  const baselineTime = new Date(baseline.created_at).getTime();
  return reviewTime > baselineTime || (reviewTime === baselineTime && review.id > baseline.id);
}

function reconcile(identity: string, requestedReviewers: RequestedReviewer[], reviews: PRReview[]) {
  let removed = pruneExpired();
  const requestedLogins = new Set(
    requestedReviewers.map((reviewer) => normalizeLogin(reviewer.login)),
  );
  for (const [key, entry] of entries) {
    if (entry.identity !== identity) continue;
    if (
      requestedLogins.has(entry.reviewer) ||
      supersedesBaseline(latestReview(reviews, entry.reviewer), entry.baseline)
    ) {
      entries.delete(key);
      removed = true;
    }
  }
  rescheduleExpiryTimer();
  if (removed) notify();
}

function beginRequest(identity: string, reviewer: string, baseline: ReviewBaseline | null) {
  const expired = pruneExpired();
  const key = entryKey(identity, reviewer);
  if (entries.has(key)) {
    if (expired) {
      rescheduleExpiryTimer();
      notify();
    }
    return null;
  }
  const operationId = ++nextOperationId;
  entries.set(key, {
    identity,
    reviewer: normalizeLogin(reviewer),
    baseline,
    status: "requesting",
    operationId,
    expiresAt: null,
  });
  rescheduleExpiryTimer();
  notify();
  return operationId;
}

function finishRequest(
  identity: string,
  reviewer: string,
  operationId: number,
  succeeded: boolean,
) {
  const key = entryKey(identity, reviewer);
  const entry = entries.get(key);
  if (!entry || entry.operationId !== operationId) return;
  if (!succeeded) {
    entries.delete(key);
  } else {
    entries.set(key, { ...entry, status: "optimistic", expiresAt: Date.now() + ENTRY_TTL_MS });
  }
  rescheduleExpiryTimer();
  notify();
}

function getEntries(identity: string) {
  return Array.from(entries.values()).filter((entry) => entry.identity === identity);
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function clearPRReviewRequestRegistryForTests() {
  entries.clear();
  nextOperationId = 0;
  rescheduleExpiryTimer();
  notify();
}

export function usePRScopedReviewRequest(
  taskPR: TaskPR,
  { workspaceId, requestedReviewers, reviews, refresh, toast }: ReviewRequestOptions,
) {
  const identity = prIdentity(workspaceId, taskPR);
  useSyncExternalStore(
    subscribe,
    () => revision,
    () => revision,
  );

  useEffect(() => {
    if (!identity) return;
    reconcile(identity, requestedReviewers, reviews);
  }, [identity, requestedReviewers, reviews]);

  const reRequest = useCallback(
    async (reviewer: string) => {
      if (!workspaceId || !identity) {
        toast({
          title: "Failed to re-request review",
          description: "Select a workspace before requesting review.",
          variant: "error",
        });
        return;
      }
      const baseline = latestReview(reviews, reviewer);
      const operationId = beginRequest(identity, reviewer, baseline);
      if (operationId === null) return;
      try {
        await requestPRReviewers(
          taskPR.owner,
          taskPR.repo,
          taskPR.pr_number,
          [reviewer],
          workspaceId,
        );
      } catch (error) {
        finishRequest(identity, reviewer, operationId, false);
        toast({
          title: "Failed to re-request review",
          description: error instanceof Error ? error.message : "An error occurred",
          variant: "error",
        });
        return;
      }
      finishRequest(identity, reviewer, operationId, true);
      toast({ description: `Review re-requested from ${reviewer}`, variant: "success" });
      try {
        refresh();
      } catch {
        // The request remains optimistic until the bounded registry expires or server data resolves it.
      }
    },
    [identity, refresh, reviews, taskPR.owner, taskPR.pr_number, taskPR.repo, toast, workspaceId],
  );

  const activeEntries = identity ? getEntries(identity) : [];
  const requestingReviewers = activeEntries
    .filter((entry) => entry.status === "requesting")
    .map((entry) => entry.reviewer);
  const optimisticReviewers = activeEntries
    .filter((entry) => entry.status === "optimistic")
    .filter(
      (entry) =>
        !requestedReviewers.some((reviewer) => normalizeLogin(reviewer.login) === entry.reviewer),
    )
    .map((entry) => ({ login: entry.reviewer, type: "user" as const }));

  return {
    reRequest,
    requestingReviewers,
    requestedReviewers: [...requestedReviewers, ...optimisticReviewers],
  };
}
