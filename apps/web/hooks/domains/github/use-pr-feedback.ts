"use client";

import { useState, useEffect, useCallback, useReducer } from "react";
import { getPRFeedback } from "@/lib/api/domains/github-api";
import type { PRFeedback } from "@/lib/types/github";

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

type Action =
  | { type: "fetch"; key: string }
  | { type: "success"; key: string; feedback: PRFeedback }
  | { type: "error"; key: string; message: string };

const INITIAL_STATE: PRFeedbackState = { key: "", feedback: null, loading: false, error: null };

function reducer(state: PRFeedbackState, action: Action): PRFeedbackState {
  // Same-PR refresh keeps the cached feedback visible while it reloads;
  // a different PR (e.g. a task switch reusing this hook instance) never
  // inherits another PR's feedback.
  const carried = state.key === action.key ? state.feedback : null;
  switch (action.type) {
    case "fetch":
      return { key: action.key, feedback: carried, loading: true, error: null };
    case "success":
      return { key: action.key, feedback: action.feedback, loading: false, error: null };
    case "error":
      return { key: action.key, feedback: carried, loading: false, error: action.message };
  }
}

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
  const [state, dispatch] = useReducer(reducer, INITIAL_STATE);
  const [fetchCount, setFetchCount] = useState(0);
  const key =
    workspaceId && owner && repo && prNumber ? `${workspaceId}/${owner}/${repo}/${prNumber}` : "";

  const refresh = useCallback(() => {
    setFetchCount((c) => c + 1);
  }, []);

  useEffect(() => {
    if (!workspaceId || !owner || !repo || !prNumber || !key) return;
    let cancelled = false;
    dispatch({ type: "fetch", key });
    getPRFeedback(workspaceId, owner, repo, prNumber, { cache: "no-store" })
      .then((response) => {
        if (!cancelled) dispatch({ type: "success", key, feedback: response });
      })
      .catch((err) => {
        if (!cancelled)
          dispatch({
            type: "error",
            key,
            message: err instanceof Error ? err.message : "Failed to fetch PR feedback",
          });
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, owner, repo, prNumber, key, fetchCount]);

  return { ...resolvePRFeedbackView(state, key), refresh };
}
