"use client";

import { useEffect, useCallback } from "react";
import { getPRFeedback } from "@/lib/api/domains/github-api";
import type { PRFeedback } from "@/lib/types/github";
import { t } from "@/lib/i18n";
import { usePRFeedbackResourceScope, usePRFeedbackResourceSnapshot } from "./pr-feedback-resource";

export type PRFeedbackState = {
  /** `<workspaceId>/<owner>/<repo>/<prNumber>` of the request `feedback` belongs to. */
  key: string;
  feedback: PRFeedback | null;
  loading: boolean;
  error: string | null;
};

type PRFeedbackView = {
  feedback: PRFeedback | null;
  loading: boolean;
  error: string | null;
};

/**
 * Derives the feedback actually safe to render for `requestedKey`. Masks out
 * `state.feedback` whenever it belongs to a different PR than the one
 * currently requested — this runs on every render (not just after the fetch
 * effect dispatches), so a PR/task switch stops showing the previous PR's
 * reviews/comments immediately, before the new fetch even resolves.
 */
export function resolvePRFeedbackView(
  state: PRFeedbackState,
  requestedKey: string,
): PRFeedbackView {
  if (state.key === requestedKey) {
    return { feedback: state.feedback, loading: state.loading, error: state.error };
  }
  return { feedback: null, loading: requestedKey !== "", error: null };
}

/**
 * Fetch live PR feedback (reviews, comments, checks) from GitHub.
 * This is not stored in the global store since it's session-scoped
 * and fetched on demand.
 */
export function usePRFeedback(
  workspaceId: string | null,
  owner: string | null,
  repo: string | null,
  prNumber: number | null,
) {
  const scope = usePRFeedbackResourceScope();
  const key =
    workspaceId && owner && repo && prNumber
      ? scope.key({ workspaceId, owner, repo, prNumber })
      : null;
  const snapshot = usePRFeedbackResourceSnapshot(scope, key);
  const fetch = useCallback(() => {
    if (!workspaceId || !owner || !repo || !prNumber) return Promise.reject();
    return getPRFeedback(workspaceId, owner, repo, prNumber, { cache: "no-store" });
  }, [owner, prNumber, repo, workspaceId]);

  useEffect(() => {
    if (key) void scope.ensure(key, fetch);
  }, [fetch, key, scope]);

  const refresh = useCallback(() => {
    if (key) void scope.invalidate(key, fetch);
  }, [fetch, key, scope]);

  const state: PRFeedbackState = {
    key: key ?? "",
    feedback: snapshot.feedback,
    loading: snapshot.loading,
    error: resolveFeedbackError(snapshot.error),
  };

  return { ...resolvePRFeedbackView(state, key ?? ""), refresh };
}

function resolveFeedbackError(error: unknown): string | null {
  if (error instanceof Error) return error.message;
  if (error) return t("github:failedToFetchPrFeedback");
  return null;
}
